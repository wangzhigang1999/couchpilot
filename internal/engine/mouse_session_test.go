package engine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestOverlappingMouseBindingsShareOneHold(t *testing.T) {
	for _, action := range []core.Action{core.ClickLeft, core.ClickRight} {
		for _, ending := range []struct {
			name       string
			remaining  core.Button
			disconnect bool
		}{
			{"release first button", core.X, false},
			{"release second button", core.A, false},
			{"release together", 0, false},
			{"disconnect", 0, true},
		} {
			t.Run(string(action)+"/"+ending.name, func(t *testing.T) {
				options := defaultOptions()
				options.Bindings = map[string]map[string]string{"default": {"a": string(action), "x": string(action)}}
				desktop := &fakeDesktop{}
				controller := New(options, fakeGamepad{}, desktop, false, nil)
				step := func(buttons core.Button) {
					t.Helper()
					if err := controller.Step(core.State{Buttons: buttons}, 0, time.Unix(100, 0)); err != nil {
						t.Fatal(err)
					}
				}
				down, up, _ := heldActionPair(action)
				step(core.A)
				step(core.A | core.X)
				if !reflect.DeepEqual(desktop.actions, []core.Action{core.Action(down)}) {
					t.Errorf("overlapping press duplicated mouse down: %v", desktop.actions)
				}
				if ending.disconnect {
					if err := controller.disconnect(); err != nil {
						t.Fatal(err)
					}
				} else {
					step(ending.remaining)
					if ending.remaining != 0 {
						for _, operation := range desktop.actions {
							if operation == core.Action(up) {
								t.Fatalf("released mouse while another button remained held: %v", desktop.actions)
							}
						}
					}
					step(0)
				}
				if err := controller.shutdown(); err != nil {
					t.Fatal(err)
				}
				want := []core.Action{core.Action(down), core.Action(up)}
				if !reflect.DeepEqual(desktop.actions, want) || len(controller.held.mouse) != 0 {
					t.Fatalf("actions=%v pending=%v, want %v and no pending release", desktop.actions, controller.held.mouse, want)
				}
			})
		}
	}
}

func TestSharedMouseHoldRetainsFailedFinalRelease(t *testing.T) {
	failure := errors.New("mouse release failed")
	attempts := 0
	desktop := &fakeDesktop{performHook: func(action core.Action, _ int) error {
		if action == core.Action(core.MouseLeftUp) {
			attempts++
			if attempts == 1 {
				return failure
			}
		}
		return nil
	}}
	options := defaultOptions()
	options.Bindings = map[string]map[string]string{"default": {"x": string(core.ClickLeft)}}
	controller := New(options, fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{Buttons: core.A | core.X}, 0, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := controller.Step(core.State{}, 0, time.Unix(101, 0)); !errors.Is(err, failure) {
		t.Fatalf("release error = %v", err)
	}
	if len(controller.held.mouse) != 1 {
		t.Fatalf("pending releases = %v, want only the final owner", controller.held.mouse)
	}
	if err := controller.shutdown(); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(controller.held.mouse) != 0 {
		t.Fatalf("attempts=%d pending=%v", attempts, controller.held.mouse)
	}
}

func TestDifferentMouseButtonsRemainIndependent(t *testing.T) {
	desktop := &fakeDesktop{}
	controller := New(defaultOptions(), fakeGamepad{}, desktop, false, nil)
	for _, buttons := range []core.Button{core.A, core.A | core.X, core.X, 0} {
		if err := controller.Step(core.State{Buttons: buttons}, 0, time.Unix(100, 0)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.Action(core.MouseLeftDown), core.Action(core.MouseRightDown), core.Action(core.MouseLeftUp), core.Action(core.MouseRightUp)}
	if !reflect.DeepEqual(desktop.actions, want) {
		t.Fatalf("actions=%v, want %v", desktop.actions, want)
	}
}
