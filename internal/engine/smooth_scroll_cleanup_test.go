package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type failingSmoothDesktop struct {
	smoothDesktop
	endError    error
	endAttempts int
	failures    int
}

func (d *failingSmoothDesktop) ScrollSmooth(amount float64, phase core.SmoothScrollPhase) error {
	if err := d.smoothDesktop.ScrollSmooth(amount, phase); err != nil {
		return err
	}
	if phase == core.SmoothScrollEnded {
		d.endAttempts++
		if d.failures < 0 || d.endAttempts <= d.failures {
			return d.endError
		}
	}
	return nil
}

func TestNaturalScrollEndFailureRemainsPendingForCleanup(t *testing.T) {
	failure := errors.New("cannot end scroll")
	desktop := &failingSmoothDesktop{endError: failure, failures: 1}
	controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
	now := time.Unix(100, 0)
	if err := controller.Step(core.State{RightY: 1}, 0.05, now); err != nil {
		t.Fatal(err)
	}
	// Let the velocity decay below the threshold for a natural end.
	if err := controller.Step(core.State{}, 1, now.Add(time.Second)); !errors.Is(err, failure) {
		t.Fatalf("end error = %v, want %v", err, failure)
	}
	if !controller.smoothScrollActive {
		t.Fatal("failed scroll end was forgotten")
	}
	if err := controller.shutdown(); err != nil {
		t.Fatal(err)
	}
	if desktop.endAttempts != 2 || controller.smoothScrollActive {
		t.Fatalf("attempts=%d active=%t, want a successful cleanup retry", desktop.endAttempts, controller.smoothScrollActive)
	}
}

func TestDisconnectBoundsScrollEndRetriesAndPreservesFailures(t *testing.T) {
	for _, test := range []struct {
		name     string
		failures int
		attempts int
		wantErr  bool
	}{
		{"success", 0, 1, false},
		{"temporary failure", 1, 2, false},
		{"persistent failure", -1, 3, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := errors.New("cannot end scroll")
			desktop := &failingSmoothDesktop{endError: failure, failures: test.failures}
			controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
			if err := controller.Step(core.State{RightY: 1, Buttons: core.A}, 0.05, time.Unix(100, 0)); err != nil {
				t.Fatal(err)
			}
			err := controller.disconnect()
			if test.wantErr && !errors.Is(err, failure) || !test.wantErr && err != nil {
				t.Fatalf("disconnect error = %v, wantErr=%t", err, test.wantErr)
			}
			if desktop.endAttempts != test.attempts || controller.smoothScrollActive != test.wantErr {
				t.Fatalf("attempts=%d active=%t; want attempts=%d active=%t", desktop.endAttempts, controller.smoothScrollActive, test.attempts, test.wantErr)
			}
			if len(controller.held.mouse) != 0 || len(desktop.actions) != 2 || desktop.actions[1] != core.Action(core.MouseLeftUp) {
				t.Fatalf("scroll cleanup prevented mouse release: %v", desktop.actions)
			}
			desktop.failures = 0
			if err := controller.shutdown(); err != nil {
				t.Fatal(err)
			}
			wantAttempts := test.attempts
			if test.wantErr {
				wantAttempts++
			}
			if desktop.endAttempts != wantAttempts || controller.smoothScrollActive {
				t.Fatalf("recovery: attempts=%d active=%t, want attempts=%d", desktop.endAttempts, controller.smoothScrollActive, wantAttempts)
			}
		})
	}
}

func TestRunReportsScrollCleanupFailure(t *testing.T) {
	failure := errors.New("cannot end scroll")
	desktop := &failingSmoothDesktop{endError: failure, failures: -1}
	controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{RightY: 1}, 0.05, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := controller.Run(ctx); !errors.Is(err, failure) {
		t.Fatalf("run error = %v, want cleanup failure", err)
	}
	if desktop.endAttempts != 3 || !controller.smoothScrollActive {
		t.Fatalf("attempts=%d active=%t", desktop.endAttempts, controller.smoothScrollActive)
	}
}
