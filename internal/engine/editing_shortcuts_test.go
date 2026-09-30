package engine

import (
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

func TestGlobalRTEditingShortcuts(t *testing.T) {
	for _, profile := range []string{"default", "chrome", "codex", "custom"} {
		for _, test := range []struct {
			button  core.Button
			gesture string
			action  core.Action
		}{
			{core.A, "rt+a", core.Enter},
			{core.B, "rt+b", core.Backspace},
			{core.X, "rt+x", core.Escape},
			{core.Y, "rt+y", core.Find},
		} {
			for _, composing := range []bool{false, true} {
				name := profile + "/" + test.gesture
				if composing {
					name += "/after voice"
				}
				t.Run(name, func(t *testing.T) {
					desktop := &fakeDesktop{profile: profile, processName: "editor"}
					controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
					now := time.Unix(100, 0)
					step := func(state core.State, delay time.Duration) {
						t.Helper()
						if err := controller.Step(state, 0, now.Add(delay)); err != nil {
							t.Fatal(err)
						}
					}
					if composing {
						step(core.State{Buttons: core.Y}, 0)
						step(core.State{}, time.Millisecond)
						desktop.actions = nil
					}
					recorder := &fakeTraceRecorder{}
					controller.SetTraceSink(recorder)
					state := core.State{Buttons: test.button, RightTrigger: 1}
					step(state, 100*time.Millisecond) // RT+A bypasses the voice delay.
					step(state, time.Second)          // Holding an editing chord does not repeat.
					step(core.State{}, 2*time.Second)
					if !reflect.DeepEqual(desktop.actions, []core.Action{test.action}) {
						t.Fatalf("actions = %v", desktop.actions)
					}
					var attempts []trace.Fact
					for _, fact := range recorder.observations {
						if fact.Kind == trace.InputAttempt {
							attempts = append(attempts, fact)
						}
					}
					if len(attempts) != 1 || attempts[0].Gesture != test.gesture || attempts[0].Action != string(test.action) || attempts[0].Outcome != trace.Success {
						t.Fatalf("attempts = %+v", attempts)
					}
					step(core.State{Buttons: core.A}, 3*time.Second)
					if got := desktop.actions[len(desktop.actions)-1]; got != core.Action(core.MouseLeftDown) {
						t.Fatalf("stale compose session: A dispatched %q", got)
					}
				})
			}
		}
	}
}

func TestRTEditingOverridesAndDisables(t *testing.T) {
	for _, action := range []core.Action{core.QuickOpen, ""} {
		t.Run(string(action), func(t *testing.T) {
			options := defaultOptions()
			options.Bindings = map[string]map[string]string{"custom": {"rt+y": string(action)}}
			desktop := &fakeDesktop{profile: "custom"}
			controller := New(options, fakeGamepad{}, desktop, false, nil)
			if err := controller.Step(core.State{Buttons: core.Y, RightTrigger: 1}, 0, time.Now()); err != nil {
				t.Fatal(err)
			}
			if action == "" {
				if len(desktop.actions) != 0 {
					t.Fatalf("disabled chord fell back to voice: %v", desktop.actions)
				}
			} else if !reflect.DeepEqual(desktop.actions, []core.Action{action}) {
				t.Fatalf("override actions = %v", desktop.actions)
			}
		})
	}
}

func TestRTShortcutsPreservePointerBoostAndLTPriority(t *testing.T) {
	now := time.Unix(100, 0)
	run := func(state core.State) *fakeDesktop {
		t.Helper()
		desktop := &fakeDesktop{}
		if err := New(defaultOptions(), fakeGamepad{}, desktop, false, nil).Step(state, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		return desktop
	}
	normal := run(core.State{LeftX: 1})
	boosted := run(core.State{LeftX: 1, RightTrigger: 1, Buttons: core.X})
	if boosted.moves[0][0] <= normal.moves[0][0] || !reflect.DeepEqual(boosted.actions, []core.Action{core.Escape}) {
		t.Fatalf("boosted = %+v", boosted)
	}
	dual := run(core.State{Buttons: core.A, LeftTrigger: 1, RightTrigger: 1})
	if !reflect.DeepEqual(dual.actions, []core.Action{core.Action(core.MouseLeftDown)}) {
		t.Fatalf("RT unexpectedly overrides LT: %v", dual.actions)
	}
}

func TestTriggerChangesDoNotReinterpretHeldButtons(t *testing.T) {
	for _, startWithRT := range []bool{false, true} {
		desktop := &fakeDesktop{}
		controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
		first, second := core.State{Buttons: core.A}, core.State{Buttons: core.A, RightTrigger: 1}
		want := []core.Action{core.Action(core.MouseLeftDown), core.Action(core.MouseLeftUp)}
		if startWithRT {
			first, second = second, first
			want = []core.Action{core.Enter}
		}
		now := time.Unix(100, 0)
		for index, state := range []core.State{first, second, {}} {
			if err := controller.Step(state, 0, now.Add(time.Duration(index)*time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(desktop.actions, want) {
			t.Fatalf("startWithRT=%v actions=%v want=%v", startWithRT, desktop.actions, want)
		}
	}
}
