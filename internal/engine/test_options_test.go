package engine

// Explicit runtime fixture: engine tests do not depend on config persistence or
// its defaults. The composition-root tests cover Settings -> Options instead.
func defaultOptions() Options {
	return Options{
		ControllerIndex:            -1,
		PollHz:                     120,
		Deadzone:                   0.18,
		PointerMaxSpeed:            1450,
		PointerCurve:               1.7,
		PrecisionSpeedMultiplier:   0.28,
		BoostSpeedMultiplier:       1.85,
		ScrollUnitsPerSecond:       1100,
		VoiceMode:                  "tap",
		VoiceSubmitMinDelaySeconds: 2,
		VoiceSubmitTimeoutSeconds:  120,
		HapticsEnabled:             true,
		HapticStrength:             1,
		ExitHoldSeconds:            1.5,
	}
}
