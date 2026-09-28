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

// heldSession remembers releases owed to the OS. Failed operations must never
// consume this state: disconnect/shutdown need it to retry safely.
type heldSession struct {
	mouse           map[core.Button]core.Operation
	windowSwitching bool
	voice           core.Button
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
