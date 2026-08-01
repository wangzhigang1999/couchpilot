package engine

import (
	"fmt"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func (e *Engine) armCompose(profile, foregroundApp string, now time.Time) {
	if _, found := e.resolver.Resolve(profile, "voice+a"); !found {
		e.clearCompose("unsupported")
		return
	}
	e.composeProfile = profile
	e.composeForegroundApp = foregroundApp
	e.composeReadyAt = now.Add(time.Duration(e.options.VoiceSubmitMinDelaySeconds * float64(time.Second)))
	e.composeUntil = now.Add(time.Duration(e.options.VoiceSubmitTimeoutSeconds * float64(time.Second)))
	if e.verbose {
		e.logger.Printf("%s voice submit armed; A presses Enter after %.2fs", profile, e.options.VoiceSubmitMinDelaySeconds)
	}
}

func (e *Engine) composeActive(profile, foregroundApp string, now time.Time) bool {
	if e.composeProfile == "" {
		return false
	}
	if profile != e.composeProfile {
		e.clearCompose("profile_changed")
		return false
	}
	if e.composeForegroundApp != "" && foregroundApp != "" && foregroundApp != e.composeForegroundApp {
		e.clearCompose("app_changed")
		return false
	}
	if !now.Before(e.composeUntil) {
		e.clearCompose("timeout")
		return false
	}
	return true
}

func (e *Engine) expireCompose(now time.Time) {
	if e.composeProfile != "" && !now.Before(e.composeUntil) {
		e.clearCompose("timeout")
	}
}

func (e *Engine) clearCompose(reason string) {
	if e.composeProfile == "" {
		return
	}
	if e.verbose && reason != "" {
		e.logger.Printf("%s voice compose cleared: %s", e.composeProfile, reason)
	}
	e.composeProfile = ""
	e.composeForegroundApp = ""
	e.composeReadyAt = time.Time{}
	e.composeUntil = time.Time{}
	e.stopRepeat(0)
}

func (e *Engine) startRepeat(button core.Button, action core.Action, now time.Time) {
	e.repeatButton = button
	e.repeatAction = action
	e.repeatNext = now.Add(composeDeleteRepeatDelay)
}

func (e *Engine) stopRepeat(button core.Button) {
	if button != 0 && e.repeatButton != button {
		return
	}
	e.repeatButton = 0
	e.repeatAction = ""
	e.repeatNext = time.Time{}
}

func (e *Engine) repeatHeldAction(state core.State, now time.Time) error {
	if e.repeatButton == 0 || state.Buttons&e.repeatButton == 0 || now.Before(e.repeatNext) {
		return nil
	}
	profile, foregroundApp := e.foregroundContext()
	if profile != e.composeProfile {
		e.clearCompose("profile_changed")
		return nil
	}
	if e.composeForegroundApp != "" && foregroundApp != "" && foregroundApp != e.composeForegroundApp {
		e.clearCompose("app_changed")
		return nil
	}
	if err := e.desktop.Perform(e.repeatAction); err != nil {
		e.clearCompose("repeat_dispatch_failure")
		return fmt.Errorf("repeat %s: %w", e.repeatAction, err)
	}
	e.repeatNext = now.Add(composeDeleteRepeatInterval)
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
	action, found := e.heldActions[button]
	if !found {
		return nil
	}
	delete(e.heldActions, button)
	return e.desktop.PerformOperation(action)
}

func (e *Engine) releaseAllHeldActions() {
	for button := range e.heldActions {
		_ = e.releaseHeldAction(button)
	}
}

func (e *Engine) voicePressed(button core.Button) error {
	var err error
	switch e.options.VoiceMode {
	case "tap":
		err = e.desktop.PerformOperation(core.VoiceTap)
	case "toggle_while_held":
		e.voiceHeld |= button
		err = e.desktop.PerformOperation(core.VoiceTap)
	case "hold":
		e.voiceHeld |= button
		err = e.desktop.PerformOperation(core.VoiceDown)
	}
	if err == nil {
		e.actionHaptic(string(core.VoiceTap))
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
	if !e.windowSwitching {
		return nil
	}
	e.windowSwitching = false
	if err := e.desktop.PerformOperation(core.WindowCycleCommit); err != nil {
		return err
	}
	e.actionHaptic(string(core.WindowCycleCommit))
	return nil
}
