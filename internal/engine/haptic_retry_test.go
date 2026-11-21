package engine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type retryGamepad struct {
	fakeGamepad
	fail  bool
	calls [][2]uint16
}

func (g *retryGamepad) Rumble(_ core.DeviceID, left, right uint16) error {
	g.calls = append(g.calls, [2]uint16{left, right})
	if g.fail {
		return errors.New("temporary rumble failure")
	}
	return nil
}

func TestStepRetriesFailedHapticsUntilAcknowledged(t *testing.T) {
	gamepad := &retryGamepad{}
	desktop := &fakeDesktop{}
	controller := New(defaultOptions(), gamepad, desktop, false, nil)
	controller.device = "test:0"
	now := time.Unix(100, 0)
	controller.clock = &cancelOnSleepClock{now: now}
	for _, step := range []struct {
		at        time.Duration
		buttons   core.Button
		fail      bool
		wantCalls int
	}{
		{0, core.A, true, 1},                 // Failed pulse must not be acknowledged.
		{10 * time.Millisecond, 0, false, 2}, // Retry without another button press.
		{20 * time.Millisecond, 0, false, 2}, // Successful pulse is not resent.
		{45 * time.Millisecond, 0, true, 3},  // Stop at the exact click-pulse deadline.
		{55 * time.Millisecond, 0, true, 4},  // Failed stop remains pending.
		{65 * time.Millisecond, 0, false, 5}, // Keep retrying until stop succeeds.
		{75 * time.Millisecond, 0, false, 5}, // Successful stop is not resent.
	} {
		gamepad.fail = step.fail
		if err := controller.Step(core.State{Buttons: step.buttons}, 0.01, now.Add(step.at)); err != nil {
			t.Fatalf("step at %s: %v", step.at, err)
		}
		if len(gamepad.calls) != step.wantCalls {
			t.Fatalf("step at %s: rumble calls=%v, want %d calls", step.at, gamepad.calls, step.wantCalls)
		}
	}
	want := [][2]uint16{{9000, 22000}, {9000, 22000}, {}, {}, {}}
	if !reflect.DeepEqual(gamepad.calls, want) {
		t.Fatalf("rumble calls=%v, want %v", gamepad.calls, want)
	}
	wantActions := []core.Action{core.Action(core.MouseLeftDown), core.Action(core.MouseLeftUp)}
	if !reflect.DeepEqual(desktop.actions, wantActions) {
		t.Fatalf("haptic retries changed desktop actions: %v", desktop.actions)
	}
}

func TestStepDoesNotRetryExpiredUnsentPulse(t *testing.T) {
	gamepad := &retryGamepad{fail: true}
	controller := New(defaultOptions(), gamepad, &fakeDesktop{}, false, nil)
	controller.device = "test:0"
	now := time.Unix(100, 0)
	controller.clock = &cancelOnSleepClock{now: now}
	if err := controller.Step(core.State{Buttons: core.A}, 0.01, now); err != nil {
		t.Fatal(err)
	}
	gamepad.fail = false
	if err := controller.Step(core.State{}, 0.01, now.Add(45*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if want := [][2]uint16{{9000, 22000}}; !reflect.DeepEqual(gamepad.calls, want) {
		t.Fatalf("expired failed pulse was retried: %v", gamepad.calls)
	}
}
