package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestComposeSessionInvalidation(t *testing.T) {
	now := time.Unix(100, 0)
	session := composeSession{profile: "browser", foregroundApp: "chrome", until: now.Add(time.Minute)}
	for _, test := range []struct {
		name, profile, app, reason string
		at                         time.Time
	}{
		{"same context", "browser", "chrome", "", now},
		{"different profile", "editor", "chrome", "profile_changed", now},
		{"different app", "browser", "firefox", "app_changed", now},
		{"lost app identity", "browser", "", "app_changed", now},
		{"timeout boundary", "browser", "chrome", "timeout", session.until},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := session.invalidReason(test.profile, test.app, test.at); got != test.reason {
				t.Fatalf("reason = %q, want %q", got, test.reason)
			}
		})
	}
}

func TestRepeatSessionTimingAndRelease(t *testing.T) {
	now := time.Unix(100, 0)
	repeat := repeatSession{button: core.B, action: core.Backspace, next: now}
	if repeat.due(core.B, now.Add(-time.Nanosecond)) || repeat.due(0, now) || !repeat.due(core.B, now) {
		t.Fatal("invalid repeat timing")
	}
	repeat.stop(core.A)
	if !repeat.due(core.B, now) {
		t.Fatal("unrelated release stopped repeat")
	}
	repeat.stop(core.B)
	if repeat.due(core.B, now) {
		t.Fatal("repeat survived release")
	}
}

func TestNewVoiceSessionStopsPreviousRepeat(t *testing.T) {
	controller := New(defaultOptions(), fakeGamepad{}, &fakeDesktop{}, false, nil)
	now := time.Unix(100, 0)
	controller.armCompose("codex", "editor", now)
	controller.startRepeat(core.B, core.Backspace, now)
	controller.armCompose("codex", "editor", now.Add(time.Second))
	if controller.compose.repeat.due(core.B, now.Add(2*time.Second)) {
		t.Fatal("old deletion repeat survived new voice session")
	}
}

func TestRunReportsBoundedCleanupFailureAndPreservesPendingRelease(t *testing.T) {
	failure := errors.New("release unavailable")
	attempts := 0
	desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
		if action == core.Action(core.MouseLeftUp) {
			attempts++
			return failure
		}
		return nil
	}}
	controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{Buttons: core.A}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := controller.Run(ctx); !errors.Is(err, failure) {
		t.Fatalf("cleanup error = %v", err)
	}
	if attempts != 3 || len(controller.held.mouse) != 1 {
		t.Fatalf("attempts=%d pending=%v", attempts, controller.held.mouse)
	}
	desktop.performHook = nil
	if err := controller.shutdown(); err != nil {
		t.Fatal(err)
	}
	if len(controller.held.mouse) != 0 {
		t.Fatal("release not consumed on recovery")
	}
}

func TestDisconnectRetriesVoiceRelease(t *testing.T) {
	options := defaultOptions()
	options.VoiceMode = "hold"
	attempts := 0
	desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
		if action == core.Action(core.VoiceUp) {
			attempts++
			if attempts == 1 {
				return errors.New("temporary release failure")
			}
		}
		return nil
	}}
	controller := New(options, fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{Buttons: core.Y}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := controller.disconnect(); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || controller.held.voice.buttons != 0 {
		t.Fatalf("attempts=%d held=%v", attempts, controller.held.voice)
	}
}

func TestCleanupDoesNotBlindlyRepeatVoiceToggle(t *testing.T) {
	options := defaultOptions()
	options.VoiceMode = "toggle_while_held"
	taps := 0
	failure := errors.New("toggle outcome uncertain")
	desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
		if action == core.Action(core.VoiceTap) {
			taps++
			if taps > 1 {
				return failure
			}
		}
		return nil
	}}
	controller := New(options, fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{Buttons: core.Y}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := controller.disconnect(); !errors.Is(err, failure) {
		t.Fatalf("cleanup error = %v", err)
	}
	if taps != 2 {
		t.Fatalf("non-idempotent toggle retried: %d taps", taps)
	}
}

func TestIdleContextLookupOnlyWhileComposing(t *testing.T) {
	desktop := &fakeDesktop{profile: "default", processName: "editor"}
	controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
	now := time.Unix(100, 0)
	if err := controller.Step(core.State{}, 0, now); err != nil {
		t.Fatal(err)
	}
	if desktop.contextCalls != 0 {
		t.Fatal("idle engine queried foreground app")
	}
	if err := controller.Step(core.State{Buttons: core.Y}, 0, now); err != nil {
		t.Fatal(err)
	}
	if err := controller.Step(core.State{}, 0, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if desktop.contextCalls != 2 {
		t.Fatalf("context lookups = %d, want one per composing frame", desktop.contextCalls)
	}
}

func TestEmergencyExitPreservesCleanupFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "release failed"}[fail], func(t *testing.T) {
			failure := errors.New("mouse release failed")
			desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
				if fail && action == core.Action(core.MouseLeftUp) {
					return failure
				}
				return nil
			}}
			gamepad := &multiGamepad{devices: []core.DeviceID{"test:0"}, states: map[core.DeviceID]core.State{"test:0": {Buttons: core.Back | core.Start}}}
			controller := New(defaultOptions(), gamepad, desktop, false, nil)
			if err := controller.Step(core.State{Buttons: core.A}, 0, time.Now()); err != nil {
				t.Fatal(err)
			}
			controller.exitComboStarted = time.Now().Add(-time.Minute)
			err := controller.Run(context.Background())
			if !errors.Is(err, ErrExitRequested) {
				t.Fatalf("exit error = %v", err)
			}
			if fail {
				if !errors.Is(err, failure) || err == ErrExitRequested {
					t.Fatalf("cleanup failure hidden: %v", err)
				}
			} else if err != ErrExitRequested {
				t.Fatalf("clean exit = %v", err)
			}
		})
	}
}
