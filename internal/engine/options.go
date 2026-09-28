package engine

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
