package main

import (
	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/engine"
)

func runtimeOptions(settings config.Settings) engine.Options {
	return engine.Options{
		DeviceID:                   settings.DeviceID,
		ControllerIndex:            settings.ControllerIndex,
		PollHz:                     settings.PollHz,
		Deadzone:                   settings.Deadzone,
		PointerMaxSpeed:            settings.PointerMaxSpeed,
		PointerCurve:               settings.PointerCurve,
		PrecisionSpeedMultiplier:   settings.PrecisionSpeedMultiplier,
		BoostSpeedMultiplier:       settings.BoostSpeedMultiplier,
		ScrollUnitsPerSecond:       settings.ScrollUnitsPerSecond,
		VoiceMode:                  settings.VoiceMode,
		VoiceSubmitMinDelaySeconds: settings.VoiceSubmitMinDelaySeconds,
		VoiceSubmitTimeoutSeconds:  settings.VoiceSubmitTimeoutSeconds,
		HapticsEnabled:             settings.HapticsEnabled,
		HapticStrength:             settings.HapticStrength,
		ExitHoldSeconds:            settings.ExitHoldSeconds,
		Bindings:                   settings.Bindings,
	}
}
