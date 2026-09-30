package engine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestShutdownRetriesFailedRelease(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   core.State
		release core.Operation
	}{
		{"mouse", core.State{Buttons: core.A}, core.MouseLeftUp},
		{"window", core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, core.WindowCycleCommit},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			failure := errors.New("temporary release failure")
			desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
				if action == core.Action(test.release) {
					attempts++
					if attempts == 1 {
						return failure
					}
				}
				return nil
			}}
			controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
			now := time.Unix(100, 0)
			if err := controller.Step(test.state, 0, now); err != nil {
				t.Fatal(err)
			}
			if err := controller.Step(core.State{}, 0, now.Add(time.Second)); !errors.Is(err, failure) {
				t.Fatalf("release error = %v", err)
			}
			controller.shutdown()
			if attempts != 2 {
				t.Fatalf("release attempts = %d, want 2", attempts)
			}
			controller.shutdown()
			if attempts != 2 {
				t.Fatalf("successful release repeated: %d", attempts)
			}
		})
	}
}

func TestComposeCancelledAfterIdleApplicationRoundTrip(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome", processName: "chrome.exe"}
	controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
	now := time.Unix(100, 0)
	step := func(button core.Button, seconds int) {
		t.Helper()
		if err := controller.Step(core.State{Buttons: button}, 0, now.Add(time.Duration(seconds)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	step(core.Y, 0)
	step(0, 1)
	desktop.profile, desktop.processName = "default", "notes.exe"
	step(0, 3)
	desktop.profile, desktop.processName = "chrome", "chrome.exe"
	step(core.A, 4)
	want := []core.Action{core.Action(core.VoiceTap), core.Action(core.MouseLeftDown)}
	if !reflect.DeepEqual(desktop.actions, want) {
		t.Fatalf("actions = %v, want %v", desktop.actions, want)
	}
}

func TestComposeBindingsUseNormalDispatch(t *testing.T) {
	for _, test := range []struct {
		action core.Action
		want   []core.Action
	}{
		{core.Voice, []core.Action{core.Action(core.VoiceTap)}},
		{core.Enter, []core.Action{core.Enter}},
		{core.ClickLeft, []core.Action{core.Action(core.MouseLeftDown), core.Action(core.MouseLeftUp)}},
		{core.Backspace, []core.Action{core.Backspace, core.Backspace}},
	} {
		t.Run(string(test.action), func(t *testing.T) {
			settings := defaultOptions()
			settings.Bindings = map[string]map[string]string{"codex": {"voice+b": string(test.action)}}
			desktop := &fakeDesktop{profile: "codex", performHook: func(action core.Action, _ int) error {
				if action == core.Voice {
					return errors.New("voice must be expanded to an operation")
				}
				return nil
			}}
			controller := New(settings, fakeGamepad{}, desktop, false, nil)
			now := time.Unix(100, 0)
			for i, button := range []core.Button{core.Y, 0, core.B, core.B, 0} {
				if err := controller.Step(core.State{Buttons: button}, 0, now.Add(time.Duration(i)*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if got := desktop.actions[1:]; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("actions = %v, want %v", got, test.want)
			}
		})
	}
}
