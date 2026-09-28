package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/engine"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
)

func TestRuntimeOptionsCopiesOnlyRuntimeConfiguration(t *testing.T) {
	settings := config.Default()
	settings.DeviceID = "controller:7"
	settings.ControllerIndex = 3
	settings.PollHz = 240
	settings.Deadzone = 0.21
	settings.PointerMaxSpeed = 1700
	settings.PointerCurve = 1.9
	settings.PrecisionSpeedMultiplier = 0.3
	settings.BoostSpeedMultiplier = 2.1
	settings.ScrollUnitsPerSecond = 1250
	settings.VoiceMode = "hold"
	settings.VoiceSubmitMinDelaySeconds = 2.5
	settings.VoiceSubmitTimeoutSeconds = 90
	settings.HapticsEnabled = false
	settings.HapticStrength = 0.7
	settings.ExitHoldSeconds = 2
	settings.Bindings = map[string]map[string]string{"custom": {"a": "enter"}}

	got := runtimeOptions(settings)
	want := engine.Options{
		DeviceID:                   "controller:7",
		ControllerIndex:            3,
		PollHz:                     240,
		Deadzone:                   0.21,
		PointerMaxSpeed:            1700,
		PointerCurve:               1.9,
		PrecisionSpeedMultiplier:   0.3,
		BoostSpeedMultiplier:       2.1,
		ScrollUnitsPerSecond:       1250,
		VoiceMode:                  "hold",
		VoiceSubmitMinDelaySeconds: 2.5,
		VoiceSubmitTimeoutSeconds:  90,
		HapticsEnabled:             false,
		HapticStrength:             0.7,
		ExitHoldSeconds:            2,
		Bindings:                   settings.Bindings,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtimeOptions() = %#v, want %#v", got, want)
	}
}

type bindingTestDesktop struct {
	actions []core.Action
}

func (*bindingTestDesktop) MovePointer(int, int) error          { return nil }
func (*bindingTestDesktop) Scroll(int) error                    { return nil }
func (*bindingTestDesktop) ForegroundContext() (string, string) { return "default", "editor" }
func (d *bindingTestDesktop) Perform(action core.Action) error {
	d.actions = append(d.actions, action)
	return nil
}
func (d *bindingTestDesktop) PerformOperation(operation core.Operation) error {
	return d.Perform(core.Action(operation))
}

func TestSupportedBindingsDispatchFromValidatedConfiguration(t *testing.T) {
	for _, item := range mapping.ButtonGestures() {
		for _, prefix := range []string{"", "lt+", "rt+", "voice+"} {
			if prefix == "voice+" && item.Button != core.A && item.Button != core.B {
				continue
			}
			gesture := prefix + item.Gesture
			t.Run(gesture, func(t *testing.T) {
				settings := config.Default()
				settings.Bindings = map[string]map[string]string{"default": {gesture: string(core.Find)}}
				if err := settings.Validate(); err != nil {
					t.Fatal(err)
				}
				desktop := &bindingTestDesktop{}
				controller := engine.New(runtimeOptions(settings), nil, desktop, false, nil)
				now := time.Unix(100, 0)
				if prefix == "voice+" {
					if err := controller.Step(core.State{Buttons: core.Y}, 0, now); err != nil {
						t.Fatal(err)
					}
					if err := controller.Step(core.State{}, 0, now.Add(time.Second)); err != nil {
						t.Fatal(err)
					}
					desktop.actions = nil
				}
				state := core.State{Buttons: item.Button}
				if prefix == "lt+" {
					state.LeftTrigger = 1
				}
				if prefix == "rt+" {
					state.RightTrigger = 1
				}
				if err := controller.Step(state, 0, now.Add(3*time.Second)); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(desktop.actions, []core.Action{core.Find}) {
					t.Fatalf("actions = %v", desktop.actions)
				}
			})
		}
	}
}
