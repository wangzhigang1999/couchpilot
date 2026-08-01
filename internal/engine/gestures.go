package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

var gestures = []struct {
	button  core.Button
	gesture string
}{
	{core.DPadUp, "dpad_up"}, {core.DPadDown, "dpad_down"},
	{core.DPadLeft, "dpad_left"}, {core.DPadRight, "dpad_right"},
	{core.LeftShoulder, "lb"}, {core.RightShoulder, "rb"},
	{core.LeftThumb, "l3"}, {core.RightThumb, "r3"},
	{core.A, "a"}, {core.B, "b"}, {core.X, "x"}, {core.Y, "y"},
}

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
		if released&item.button == 0 {
			continue
		}
		e.stopRepeat(item.button)
		if err := e.releaseHeldAction(item.button); err != nil {
			return err
		}
	}
	if released&e.voiceHeld != 0 {
		for _, item := range gestures {
			if released&item.button != 0 && e.voiceHeld&item.button != 0 {
				if err := e.voiceReleased(); err != nil {
					return err
				}
				e.voiceHeld &^= item.button
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
		if pressed&item.button == 0 {
			continue
		}
		gesture := item.gesture
		composeActive := e.composeActive(profile, foregroundApp, now)
		noTrigger := state.LeftTrigger <= 0.08 && state.RightTrigger <= 0.08
		composeSubmit := false
		composeDelete := false
		if composeActive && noTrigger && gesture == "a" {
			if now.Before(e.composeReadyAt) {
				resolved := e.resolver.ResolveDetailed(profile, "voice+a")
				e.recordResolved(now, item.gesture, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
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
				composeDelete = true
			} else {
				e.clearCompose("other_control")
			}
		} else if composeActive && gesture != "y" {
			e.clearCompose("other_control")
		}
		baseGesture := gesture
		leftActive := !composeSubmit && state.LeftTrigger > 0.08
		rightActive := !composeSubmit && state.RightTrigger > 0.08
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
			e.recordResolved(now, item.gesture, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_unavailable")
			}
			continue
		}
		action := resolved.Action
		if composeDelete {
			if err := e.desktop.Perform(action); err != nil {
				e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
				e.clearCompose("delete_dispatch_failed")
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
			e.startRepeat(item.button, action, now)
			e.actionHaptic(string(action))
			if e.verbose {
				e.logger.Printf("%s/%s -> %s", profile, gesture, action)
			}
			return nil
		}
		if action == core.Voice {
			if err := e.voicePressed(item.button); err != nil {
				e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
				if composeSubmit {
					e.clearCompose("submit_dispatch_failed")
				}
				return err
			}
			e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_succeeded")
			}
			e.armCompose(profile, foregroundApp, now)
			continue
		}
		if down, up, held := heldActionPair(action); held {
			if err := e.desktop.PerformOperation(down); err != nil {
				e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
				if composeSubmit {
					e.clearCompose("submit_dispatch_failed")
				}
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_succeeded")
			}
			e.heldActions[item.button] = up
			e.actionHaptic(string(action))
			continue
		}
		var operation core.Operation
		if strings.HasPrefix(gesture, "lt+") {
			switch action {
			case core.WindowPrevious:
				operation = core.WindowCyclePrevious
				e.windowSwitching = true
			case core.WindowNext:
				operation = core.WindowCycleNext
				e.windowSwitching = true
			}
		}
		var dispatchErr error
		if operation != "" {
			dispatchErr = e.desktop.PerformOperation(operation)
		} else {
			dispatchErr = e.desktop.Perform(action)
		}
		if dispatchErr != nil {
			if e.windowSwitching {
				_ = e.finishWindowSwitch()
			}
			e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_dispatch_failed")
			}
			return fmt.Errorf("perform %s: %w", action, dispatchErr)
		}
		e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
		if composeSubmit {
			e.clearCompose("submit_succeeded")
		}
		hapticAction := string(action)
		if operation != "" {
			hapticAction = string(operation)
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
