package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

const SchemaVersion = 1

type Settings struct {
	SchemaVersion            int                          `json:"schema_version"`
	DeviceID                 string                       `json:"device_id,omitempty"`
	ControllerIndex          int                          `json:"controller_index"`
	PollHz                   int                          `json:"poll_hz"`
	Deadzone                 float64                      `json:"deadzone"`
	PointerMaxSpeed          float64                      `json:"pointer_max_speed"`
	PointerCurve             float64                      `json:"pointer_curve"`
	PrecisionSpeedMultiplier float64                      `json:"precision_speed_multiplier"`
	BoostSpeedMultiplier     float64                      `json:"boost_speed_multiplier"`
	ScrollUnitsPerSecond     float64                      `json:"scroll_units_per_second"`
	VoiceMode                string                       `json:"voice_mode"`
	VoiceKey                 string                       `json:"voice_key,omitempty"`
	ExitHoldSeconds          float64                      `json:"exit_hold_seconds"`
	Bindings                 map[string]map[string]string `json:"bindings,omitempty"`
}

func Default() Settings {
	return Settings{
		SchemaVersion:            SchemaVersion,
		ControllerIndex:          -1,
		PollHz:                   120,
		Deadzone:                 0.18,
		PointerMaxSpeed:          1450,
		PointerCurve:             1.7,
		PrecisionSpeedMultiplier: 0.28,
		BoostSpeedMultiplier:     1.85,
		ScrollUnitsPerSecond:     1100,
		VoiceMode:                "tap",
		VoiceKey:                 "right_alt",
		ExitHoldSeconds:          1.5,
	}
}

func Load(path string) (Settings, error) {
	settings := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := Save(path, settings); err != nil {
			return Settings{}, err
		}
		return settings, nil
	}
	if err != nil {
		return Settings{}, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("decode config: %w", err)
	}
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = SchemaVersion
	}
	return settings, settings.Validate()
}

func Save(path string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (s Settings) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", s.SchemaVersion)
	}
	if s.PollHz < 30 || s.PollHz > 1000 {
		return errors.New("poll_hz must be between 30 and 1000")
	}
	if s.Deadzone < 0 || s.Deadzone >= 0.95 {
		return errors.New("deadzone must be between 0 and 0.95")
	}
	if s.PrecisionSpeedMultiplier < 0.05 || s.PrecisionSpeedMultiplier > 1 {
		return errors.New("precision_speed_multiplier must be between 0.05 and 1")
	}
	if s.BoostSpeedMultiplier < 1 || s.BoostSpeedMultiplier > 4 {
		return errors.New("boost_speed_multiplier must be between 1 and 4")
	}
	if s.VoiceMode != "tap" && s.VoiceMode != "hold" && s.VoiceMode != "toggle_while_held" {
		return errors.New("voice_mode must be tap, hold, or toggle_while_held")
	}
	if s.ExitHoldSeconds <= 0 {
		return errors.New("exit_hold_seconds must be positive")
	}
	for profile, bindings := range s.Bindings {
		if profile == "" {
			return errors.New("binding profile cannot be empty")
		}
		for gesture, action := range bindings {
			if gesture == "" {
				return fmt.Errorf("binding gesture cannot be empty in profile %q", profile)
			}
			if action != "" && !core.IsKnownAction(core.Action(action)) {
				return fmt.Errorf("unknown action %q for %s/%s", action, profile, gesture)
			}
		}
	}
	return nil
}
