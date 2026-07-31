package engine

import (
	"reflect"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/config"
)

func TestOptionsFromSettingsCopiesOnlyRuntimeConfiguration(t *testing.T) {
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

	got := OptionsFromSettings(settings)
	want := Options{
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
		t.Fatalf("OptionsFromSettings() = %#v, want %#v", got, want)
	}
}
