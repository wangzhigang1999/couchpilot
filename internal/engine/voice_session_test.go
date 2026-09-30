package engine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestVoiceToggleFailureSurvivesCleanupCalls(t *testing.T) {
	for _, stage := range []string{"press", "release", "disconnect"} {
		t.Run(stage, func(t *testing.T) {
			options := defaultOptions()
			options.VoiceMode = "toggle_while_held"
			failure := errors.New("toggle outcome uncertain")
			taps := 0
			failAt := 2
			if stage == "press" {
				failAt = 1
			}
			desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
				if action == core.Action(core.VoiceTap) {
					taps++
					if taps == failAt {
						return failure
					}
				}
				return nil
			}}
			controller := New(options, fakeGamepad{}, desktop, false, nil)
			now := time.Unix(100, 0)
			err := controller.Step(core.State{Buttons: core.Y}, 0, now)
			if stage != "press" {
				if err != nil {
					t.Fatal(err)
				}
				if stage == "release" {
					err = controller.Step(core.State{}, 0, now.Add(time.Second))
				} else {
					err = controller.disconnect()
				}
			}
			if !errors.Is(err, failure) {
				t.Fatalf("initial error = %v, want %v", err, failure)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := controller.shutdown(); !errors.Is(err, failure) {
					t.Errorf("cleanup %d lost uncertain outcome: %v", attempt, err)
				}
			}
			if err := controller.Step(core.State{Buttons: core.Y}, 0, now.Add(2*time.Second)); !errors.Is(err, failure) {
				t.Errorf("new press lost uncertain outcome: %v", err)
			}
			if taps != failAt {
				t.Errorf("toggle repeated after uncertain outcome: %d taps, want %d", taps, failAt)
			}
		})
	}
}

func TestVoiceButtonsShareHeldSession(t *testing.T) {
	for _, mode := range []string{"hold", "toggle_while_held"} {
		for _, ending := range []string{"separate release", "simultaneous release", "disconnect"} {
			t.Run(mode+"/"+ending, func(t *testing.T) {
				options := defaultOptions()
				options.VoiceMode = mode
				options.Bindings = map[string]map[string]string{"default": {"x": "voice"}}
				desktop := &fakeDesktop{}
				controller := New(options, fakeGamepad{}, desktop, false, nil)
				now := time.Unix(100, 0)
				step := func(buttons core.Button) {
					t.Helper()
					if err := controller.Step(core.State{Buttons: buttons}, 0, now); err != nil {
						t.Fatal(err)
					}
					now = now.Add(time.Second)
				}
				start, stop := core.VoiceDown, core.VoiceUp
				if mode == "toggle_while_held" {
					start, stop = core.VoiceTap, core.VoiceTap
				}
				assertActions := func(want ...core.Action) {
					t.Helper()
					if !reflect.DeepEqual(desktop.actions, want) {
						t.Fatalf("actions = %v, want %v", desktop.actions, want)
					}
				}
				if ending == "separate release" {
					step(core.Y)
				}
				step(core.X | core.Y)
				assertActions(core.Action(start))
				if ending == "separate release" {
					step(core.X)
					assertActions(core.Action(start))
				}
				if ending == "disconnect" {
					if err := controller.disconnect(); err != nil {
						t.Fatal(err)
					}
				} else {
					step(0)
				}
				if err := controller.shutdown(); err != nil {
					t.Fatal(err)
				}
				assertActions(core.Action(start), core.Action(stop))
				step(core.Y)
				step(0)
				assertActions(core.Action(start), core.Action(stop), core.Action(start), core.Action(stop))
			})
		}
	}
}

func TestVoiceTapButtonsRemainIndependent(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			options := defaultOptions()
			options.Bindings = map[string]map[string]string{"default": {"x": "voice"}}
			desktop := &fakeDesktop{}
			if fail {
				desktop.performError = errors.New("tap failed")
			}
			controller := New(options, fakeGamepad{}, desktop, false, nil)
			err := controller.Step(core.State{Buttons: core.X | core.Y}, 0, time.Unix(100, 0))
			if (err != nil) != fail {
				t.Fatalf("press error = %v", err)
			}
			if err := controller.shutdown(); err != nil {
				t.Fatal(err)
			}
			want := []core.Action{core.Action(core.VoiceTap)}
			if !fail {
				want = append(want, core.Action(core.VoiceTap))
			}
			if !reflect.DeepEqual(desktop.actions, want) {
				t.Fatalf("actions = %v, want %v", desktop.actions, want)
			}
		})
	}
}

func TestVoiceHoldFailureCanRecover(t *testing.T) {
	for _, stage := range []string{"press", "release"} {
		t.Run(stage, func(t *testing.T) {
			options := defaultOptions()
			options.VoiceMode = "hold"
			failure := errors.New("voice input partially failed")
			failedOperation := core.VoiceDown
			if stage == "release" {
				failedOperation = core.VoiceUp
			}
			desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
				if action == core.Action(failedOperation) {
					return failure
				}
				return nil
			}}
			controller := New(options, fakeGamepad{}, desktop, false, nil)
			now := time.Unix(100, 0)
			err := controller.Step(core.State{Buttons: core.Y}, 0, now)
			if stage == "release" {
				if err != nil {
					t.Fatal(err)
				}
				err = controller.Step(core.State{}, 0, now.Add(time.Second))
			}
			if !errors.Is(err, failure) {
				t.Fatalf("input error = %v", err)
			}
			if stage == "release" {
				if err := controller.shutdown(); !errors.Is(err, failure) {
					t.Fatalf("cleanup error = %v", err)
				}
				// One down, one normal release, and three bounded cleanup attempts.
				if len(desktop.actions) != 5 || controller.held.voice.buttons == 0 {
					t.Fatalf("cleanup attempts = %v, pending = %v", desktop.actions, controller.held.voice)
				}
			}
			desktop.performHook = nil
			if err := controller.shutdown(); err != nil {
				t.Fatal(err)
			}
			if controller.held.voice.buttons != 0 || desktop.actions[len(desktop.actions)-1] != core.Action(core.VoiceUp) {
				t.Fatalf("voice not released: actions = %v, pending = %v", desktop.actions, controller.held.voice)
			}
			completed := len(desktop.actions)
			if err := controller.shutdown(); err != nil || len(desktop.actions) != completed {
				t.Fatalf("successful release repeated: actions = %v, error = %v", desktop.actions, err)
			}
		})
	}
}
