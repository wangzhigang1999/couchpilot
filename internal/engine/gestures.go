package engine

import (
	"fmt"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

var gestures = mapping.ButtonGestures()

var unboundSystemButtons = []struct {
	button  core.Button
	control string
}{
	{core.Back, "back"},
	{core.Start, "start"},
}

func (e *Engine) buttons(state core.State, now time.Time) error {
	pressed := core.Button(uint16(state.Buttons) &^ uint16(e.previousButtons))
	released := core.Button(uint16(e.previousButtons) &^ uint16(state.Buttons))
	if e.verbose && (pressed != 0 || released != 0) {
		e.logger.Printf("buttons: state=0x%04X pressed=0x%04X released=0x%04X", uint16(state.Buttons), uint16(pressed), uint16(released))
	}
	for _, item := range gestures {
		if released&item.Button == 0 {
			continue
		}
		e.stopRepeat(item.Button)
		if err := e.releaseHeldAction(item.Button); err != nil {
			return err
		}
	}
	if released&e.held.voice != 0 {
		for _, item := range gestures {
			if released&item.Button != 0 && e.held.voice&item.Button != 0 {
				if err := e.voiceReleased(); err != nil {
					return err
				}
				e.held.voice &^= item.Button
			}
		}
	}
	if pressed == 0 {
		return nil
	}
	profile, foregroundApp := e.foregroundContext()
	simultaneous := pressedButtonCount(pressed) > 1
	for _, item := range unboundSystemButtons {
		if pressed&item.button == 0 {
			continue
		}
		resolved := mapping.ResolvedBinding{
			ActiveProfile: profile,
			Gesture:       item.control,
			Resolution:    mapping.BindingUnbound,
		}
		e.recordResolved(now, item.control, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
	}
	for _, item := range gestures {
		if pressed&item.Button == 0 {
			continue
		}
		gesture := item.Gesture
		composeActive := e.composeActive(profile, foregroundApp, now)
		noTrigger := state.LeftTrigger <= 0.08 && state.RightTrigger <= 0.08
		composeSubmit := false
		composeEdit := false
		if composeActive && noTrigger && gesture == "a" {
			if now.Before(e.compose.readyAt) {
				resolved := e.resolver.ResolveDetailed(profile, "voice+a")
				e.recordResolved(now, item.Gesture, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
				if e.verbose {
					e.logger.Printf("%s voice submit ignored during minimum delay", profile)
				}
				continue
			}
			gesture = "voice+a"
			composeSubmit = true
		} else if composeActive && gesture == "b" {
			if _, found := e.resolver.Resolve(profile, "voice+b"); found {
				gesture = "voice+b"
				composeEdit = true
			} else {
				e.clearCompose("other_control")
			}
		} else if composeActive && gesture != "y" {
			e.clearCompose("other_control")
		}
		baseGesture := gesture
		leftActive := !composeSubmit && !composeEdit && state.LeftTrigger > 0.08
		rightActive := !composeSubmit && !composeEdit && state.RightTrigger > 0.08
		if leftActive {
			leftCandidate := e.resolver.ResolveDetailed(profile, "lt+"+baseGesture)
			if leftCandidate.Resolution == mapping.BindingBound {
				gesture = leftCandidate.Gesture
			}
		}
		if rightActive {
			rightCandidate := e.resolver.ResolveDetailed(profile, "rt+"+baseGesture)
			if !leftActive && rightCandidate.Resolution == mapping.BindingBound {
				gesture = rightCandidate.Gesture
			}
		}
		resolved := e.resolver.ResolveDetailed(profile, gesture)
		if resolved.Resolution != mapping.BindingBound {
			e.recordResolved(now, item.Gesture, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_unavailable")
			}
			continue
		}
		action := resolved.Action
		hapticAction, err := e.dispatchAction(item.Button, gesture, action)
		if err != nil {
			e.recordResolved(now, item.Gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
			if composeSubmit || composeEdit {
				e.clearCompose("compose_dispatch_failed")
			}
			return fmt.Errorf("perform %s: %w", action, err)
		}
		e.recordResolved(now, item.Gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
		if composeSubmit {
			e.clearCompose("submit_succeeded")
		}
		if action == core.Voice {
			e.armCompose(profile, foregroundApp, now)
		}
		if composeEdit && action == core.Backspace {
			e.startRepeat(item.Button, action, now)
		}
		e.actionHaptic(hapticAction)
		if e.verbose {
			e.logger.Printf("%s/%s -> %s", profile, gesture, action)
		}
		if composeSubmit {
			return nil
		}
	}
	return nil
}
