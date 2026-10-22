package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type diagnosticGamepad struct {
	core.Gamepad
	device       core.DeviceID
	calls        [][2]uint16
	startError   error
	stopError    error
	stopFailures int
}

func (g *diagnosticGamepad) Rumble(device core.DeviceID, left, right uint16) error {
	if device != g.device {
		return errors.New("wrong diagnostic device")
	}
	g.calls = append(g.calls, [2]uint16{left, right})
	if left != 0 || right != 0 {
		return g.startError
	}
	if len(g.calls)-1 <= g.stopFailures {
		return g.stopError
	}
	return nil
}

func TestControllerHapticsStopsPulseAndReportsFailures(t *testing.T) {
	failure := errors.New("controller I/O failed")
	for _, test := range []struct {
		name         string
		startError   error
		stopFailures int
		stopAttempts int
		wantError    bool
	}{
		{"success", nil, 0, 1, false},
		{"temporary stop failure", nil, 1, 2, false},
		{"persistent stop failure", nil, 100, 3, true},
		{"start failure", failure, 0, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gamepad := &diagnosticGamepad{
				device: "test:diagnostic", startError: test.startError,
				stopError: failure, stopFailures: test.stopFailures,
			}
			var sleeps []time.Duration
			err := testControllerHaptics(gamepad, gamepad.device, func(duration time.Duration) {
				sleeps = append(sleeps, duration)
			})
			if test.wantError && !errors.Is(err, failure) || !test.wantError && err != nil {
				t.Errorf("error=%v, wantError=%t", err, test.wantError)
			}
			if test.wantError && test.startError == nil && (err == nil || !strings.Contains(err.Error(), "stop") || !strings.Contains(err.Error(), string(gamepad.device))) {
				t.Errorf("stop failure lacks operation/device context: %v", err)
			}
			if len(gamepad.calls) != test.stopAttempts+1 {
				t.Fatalf("rumble calls=%v, want one start and %d stop attempts", gamepad.calls, test.stopAttempts)
			}
			if gamepad.calls[0] != [2]uint16{36000, 26000} {
				t.Errorf("pulse strength changed: %v", gamepad.calls[0])
			}
			for _, call := range gamepad.calls[1:] {
				if call != [2]uint16{} {
					t.Errorf("stop retry restarted the pulse: %v", call)
				}
			}
			if test.startError != nil {
				if len(sleeps) != 0 {
					t.Errorf("slept after failed start: %v", sleeps)
				}
			} else if len(sleeps) != 1 || sleeps[0] != 180*time.Millisecond {
				t.Errorf("pulse timing changed: %v", sleeps)
			}
		})
	}
}
