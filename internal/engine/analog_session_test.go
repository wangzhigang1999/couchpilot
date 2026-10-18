package engine

import (
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type wheelDesktop struct {
	fakeDesktop
	wheels []int
}

func (d *wheelDesktop) Scroll(amount int) error {
	d.wheels = append(d.wheels, amount)
	return nil
}

func TestAnalogRemaindersAreScopedToControllerSession(t *testing.T) {
	for _, test := range []struct {
		name   string
		state  core.State
		moves  [][2]int
		wheels []int
	}{
		{"pointer right", core.State{LeftX: 1}, [][2]int{{1, 0}}, nil},
		{"pointer left", core.State{LeftX: -1}, [][2]int{{-1, 0}}, nil},
		{"pointer up", core.State{LeftY: 1}, [][2]int{{0, -1}}, nil},
		{"pointer down", core.State{LeftY: -1}, [][2]int{{0, 1}}, nil},
		{"scroll up", core.State{RightY: 1}, nil, []int{120}},
		{"scroll down", core.State{RightY: -1}, nil, []int{-120}},
	} {
		for _, disconnect := range []bool{false, true} {
			session := "same session"
			if disconnect {
				session = "new session"
			}
			t.Run(test.name+"/"+session, func(t *testing.T) {
				options := defaultOptions()
				// Each frame contributes 0.75 pixels or 90 wheel units, below
				// the output threshold; two frames must produce one event.
				options.PointerMaxSpeed = 15
				options.ScrollUnitsPerSecond = 1800
				desktop := &wheelDesktop{}
				controller := New(options, fakeGamepad{}, desktop, false, nil)
				now := time.Unix(100, 0)
				step := func() {
					t.Helper()
					now = now.Add(50 * time.Millisecond)
					if err := controller.Step(test.state, 0.05, now); err != nil {
						t.Fatal(err)
					}
				}
				assertNoOutput := func() {
					t.Helper()
					if len(desktop.moves) != 0 || len(desktop.wheels) != 0 {
						t.Fatalf("unexpected output: moves=%v wheels=%v", desktop.moves, desktop.wheels)
					}
				}
				step()
				assertNoOutput()
				if disconnect {
					// Run uses this boundary for both unplug/reconnect and takeover.
					if err := controller.disconnect(); err != nil {
						t.Fatal(err)
					}
					step()
					assertNoOutput()
				}
				step()
				if !reflect.DeepEqual(desktop.moves, test.moves) || !reflect.DeepEqual(desktop.wheels, test.wheels) {
					t.Fatalf("moves=%v wheels=%v; want moves=%v wheels=%v", desktop.moves, desktop.wheels, test.moves, test.wheels)
				}
			})
		}
	}
}
