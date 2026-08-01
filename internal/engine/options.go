package engine

import (
	"io"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
)

// Options is the engine's runtime configuration. Persistence-only concerns,
// including app matching and trace file policy, stay outside the engine.
type Options struct {
	DeviceID                   string
	ControllerIndex            int
	PollHz                     int
	Deadzone                   float64
	PointerMaxSpeed            float64
	PointerCurve               float64
	PrecisionSpeedMultiplier   float64
	BoostSpeedMultiplier       float64
	ScrollUnitsPerSecond       float64
	VoiceMode                  string
	VoiceSubmitMinDelaySeconds float64
	VoiceSubmitTimeoutSeconds  float64
	HapticsEnabled             bool
	HapticStrength             float64
	ExitHoldSeconds            float64
	Bindings                   map[string]map[string]string
}

func OptionsFromSettings(settings config.Settings) Options {
	return Options{
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

// New is kept as a compatibility seam for existing callers and tests.
func New(settings config.Settings, gamepad core.Gamepad, desktop core.Desktop, verbose bool, output io.Writer) *Engine {
	return NewWithOptions(OptionsFromSettings(settings), gamepad, desktop, verbose, output)
}
