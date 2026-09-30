package engine

import (
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

// composeSession owns only transient voice-editing state, not platform I/O.
type composeSession struct {
	profile       string
	foregroundApp string
	readyAt       time.Time
	until         time.Time
	repeat        repeatSession
}

func (s composeSession) invalidReason(profile, app string, now time.Time) string {
	if s.profile == "" {
		return ""
	}
	if !now.Before(s.until) {
		return "timeout"
	}
	if profile != s.profile {
		return "profile_changed"
	}
	if s.foregroundApp != "" && app != s.foregroundApp {
		return "app_changed"
	}
	return ""
}

type repeatSession struct {
	button core.Button
	action core.Action
	next   time.Time
}

func (r *repeatSession) stop(button core.Button) {
	if button == 0 || r.button == button {
		*r = repeatSession{}
	}
}

func (r repeatSession) due(buttons core.Button, now time.Time) bool {
	return r.button != 0 && buttons&r.button != 0 && !now.Before(r.next)
}

// heldSession remembers releases owed to the OS, including uncertain outcomes
// that cleanup must report without retrying non-idempotent operations.
type heldSession struct {
	mouse           map[core.Button]core.Operation
	windowSwitching bool
	voice           voiceSession
}

// voiceSession shares one OS voice session between all held voice buttons.
// A failed toggle has an unknown outcome, so its error survives every cleanup
// call. Unlike key-up, toggling again is not a safe recovery operation.
type voiceSession struct {
	buttons          core.Button
	releaseOperation core.Operation
	uncertainErr     error
}

func (s *voiceSession) press(button core.Button, mode string, perform func(core.Operation) error) error {
	if s.uncertainErr != nil {
		return s.uncertainErr
	}
	var start, stop core.Operation
	switch mode {
	case "tap":
		return perform(core.VoiceTap)
	case "hold":
		start, stop = core.VoiceDown, core.VoiceUp
	case "toggle_while_held":
		start, stop = core.VoiceTap, core.VoiceTap
	default:
		return nil
	}
	if s.buttons != 0 {
		s.buttons |= button
		return nil
	}
	// Record the owed release before I/O: key-down may partially succeed.
	s.buttons, s.releaseOperation = button, stop
	err := perform(start)
	if start == core.VoiceTap {
		s.uncertainErr = err
	}
	return err
}

func (s *voiceSession) release(buttons core.Button, perform func(core.Operation) error) error {
	if s.uncertainErr != nil {
		return s.uncertainErr
	}
	if s.buttons&buttons == 0 {
		return nil
	}
	if remaining := s.buttons &^ buttons; remaining != 0 {
		s.buttons = remaining
		return nil
	}
	if err := perform(s.releaseOperation); err != nil {
		if s.releaseOperation == core.VoiceTap {
			s.uncertainErr = err
		}
		return err
	}
	*s = voiceSession{}
	return nil
}

func (s *heldSession) releaseMouse(button core.Button, perform func(core.Operation) error) error {
	operation, found := s.mouse[button]
	if !found {
		return nil
	}
	if err := perform(operation); err != nil {
		return err
	}
	delete(s.mouse, button)
	return nil
}

func (s *heldSession) commitWindow(perform func(core.Operation) error) error {
	if !s.windowSwitching {
		return nil
	}
	if err := perform(core.WindowCycleCommit); err != nil {
		return err
	}
	s.windowSwitching = false
	return nil
}
