package engine

import (
	"errors"
	"fmt"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func (e *Engine) armCompose(profile, foregroundApp string, now time.Time) {
	if _, found := e.resolver.Resolve(profile, "voice+a"); !found {
		e.clearCompose("unsupported")
		return
	}
	e.compose = composeSession{
		profile:       profile,
		foregroundApp: foregroundApp,
		readyAt:       now.Add(time.Duration(e.options.VoiceSubmitMinDelaySeconds * float64(time.Second))),
		until:         now.Add(time.Duration(e.options.VoiceSubmitTimeoutSeconds * float64(time.Second))),
	}
	if e.verbose {
		e.logger.Printf("%s voice submit armed; A presses Enter after %.2fs", profile, e.options.VoiceSubmitMinDelaySeconds)
	}
}

func (e *Engine) composeActive(profile, foregroundApp string, now time.Time) bool {
	if reason := e.compose.invalidReason(profile, foregroundApp, now); reason != "" {
		e.clearCompose(reason)
	}
	return e.compose.profile != ""
}

func (e *Engine) expireCompose(now time.Time) {
	if e.compose.profile != "" && !now.Before(e.compose.until) {
		e.clearCompose("timeout")
	}
}

func (e *Engine) clearCompose(reason string) {
	if e.compose.profile == "" {
		return
	}
	if e.verbose && reason != "" {
		e.logger.Printf("%s voice compose cleared: %s", e.compose.profile, reason)
	}
	e.compose = composeSession{}
}

func (e *Engine) startRepeat(button core.Button, action core.Action, now time.Time) {
	e.compose.repeat.button = button
	e.compose.repeat.action = action
	e.compose.repeat.next = now.Add(backspaceRepeatDelay)
}

func (e *Engine) stopRepeat(button core.Button) {
	e.compose.repeat.stop(button)
}

func (e *Engine) repeatHeldAction(state core.State, now time.Time) error {
	if !e.compose.repeat.due(state.Buttons, now) {
		return nil
	}
	profile, foregroundApp := e.foregroundContext()
	if !e.composeActive(profile, foregroundApp, now) {
		return nil
	}
	action := e.compose.repeat.action
	if err := e.desktop.Perform(action); err != nil {
		e.clearCompose("repeat_dispatch_failure")
		return fmt.Errorf("repeat %s: %w", action, err)
	}
	e.compose.repeat.next = now.Add(backspaceRepeatInterval)
	return nil
}

func heldActionPair(action core.Action) (core.Operation, core.Operation, bool) {
	switch action {
	case core.ClickLeft:
		return core.MouseLeftDown, core.MouseLeftUp, true
	case core.ClickRight:
		return core.MouseRightDown, core.MouseRightUp, true
	default:
		return "", "", false
	}
}

func (e *Engine) releaseHeldAction(button core.Button) error {
	return e.held.releaseMouse(button, e.desktop.PerformOperation)
}

// Cleanup is bounded. Only successful releases consume state, so a transient
// failure can be retried without repeating already completed operations.
func (e *Engine) releaseInputs() error {
	var err error
	var voiceErr error
	for attempt := 0; attempt < 3; attempt++ {
		err = nil
		for button := range e.held.mouse {
			err = errors.Join(err, e.releaseHeldAction(button))
		}
		err = errors.Join(err, e.finishWindowSwitch())
		if e.held.voice != 0 {
			// Key-up is safe to retry, but a voice toggle tap may have
			// already switched modes before reporting a partial failure.
			if attempt == 0 || e.options.VoiceMode == "hold" {
				voiceErr = e.voiceReleased()
				if voiceErr == nil {
					e.held.voice = 0
				}
			}
			err = errors.Join(err, voiceErr)
		}
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("release held input: %w", err)
}

func (e *Engine) voicePressed(button core.Button) error {
	var err error
	switch e.options.VoiceMode {
	case "tap":
		err = e.desktop.PerformOperation(core.VoiceTap)
	case "toggle_while_held":
		e.held.voice |= button
		err = e.desktop.PerformOperation(core.VoiceTap)
	case "hold":
		e.held.voice |= button
		err = e.desktop.PerformOperation(core.VoiceDown)
	}
	return err
}

func (e *Engine) voiceReleased() error {
	if e.options.VoiceMode == "toggle_while_held" {
		return e.desktop.PerformOperation(core.VoiceTap)
	}
	if e.options.VoiceMode == "hold" {
		return e.desktop.PerformOperation(core.VoiceUp)
	}
	return nil
}

func (e *Engine) finishWindowSwitch() error {
	if !e.held.windowSwitching {
		return nil
	}
	if err := e.held.commitWindow(e.desktop.PerformOperation); err != nil {
		return err
	}
	e.actionHaptic(string(core.WindowCycleCommit))
	return nil
}
