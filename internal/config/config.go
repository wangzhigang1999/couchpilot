package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

const SchemaVersion = 1

type Settings struct {
	SchemaVersion             int                          `json:"schema_version"`
	DeviceID                  string                       `json:"device_id,omitempty"`
	ControllerIndex           int                          `json:"controller_index"`
	PollHz                    int                          `json:"poll_hz"`
	Deadzone                  float64                      `json:"deadzone"`
	PointerMaxSpeed           float64                      `json:"pointer_max_speed"`
	PointerCurve              float64                      `json:"pointer_curve"`
	PrecisionSpeedMultiplier  float64                      `json:"precision_speed_multiplier"`
	BoostSpeedMultiplier      float64                      `json:"boost_speed_multiplier"`
	ScrollUnitsPerSecond      float64                      `json:"scroll_units_per_second"`
	VoiceMode                 string                       `json:"voice_mode"`
	VoiceKey                  string                       `json:"voice_key,omitempty"`
	VoiceSubmitTimeoutSeconds float64                      `json:"voice_submit_timeout_seconds"`
	HapticsEnabled            bool                         `json:"haptics_enabled"`
	HapticStrength            float64                      `json:"haptic_strength"`
	ExitHoldSeconds           float64                      `json:"exit_hold_seconds"`
	LocalTraceEnabled         bool                         `json:"local_trace_enabled"`
	AppProfiles               []core.AppProfile            `json:"app_profiles"`
	Bindings                  map[string]map[string]string `json:"bindings,omitempty"`
}

func Default() Settings {
	return Settings{
		SchemaVersion:             SchemaVersion,
		ControllerIndex:           -1,
		PollHz:                    120,
		Deadzone:                  0.18,
		PointerMaxSpeed:           1450,
		PointerCurve:              1.7,
		PrecisionSpeedMultiplier:  0.28,
		BoostSpeedMultiplier:      1.85,
		ScrollUnitsPerSecond:      1100,
		VoiceMode:                 "tap",
		VoiceKey:                  "platform_default",
		VoiceSubmitTimeoutSeconds: 120,
		HapticsEnabled:            true,
		HapticStrength:            1.0,
		ExitHoldSeconds:           1.5,
		LocalTraceEnabled:         true,
		AppProfiles:               defaultAppProfiles(),
	}
}

func defaultAppProfiles() []core.AppProfile {
	return []core.AppProfile{
		{Name: "codex", ProcessNames: []string{"ChatGPT.exe", "ChatGPT", "Codex"}, PathContains: []string{`\OpenAI.Codex_`, "/Codex.app/", "/ChatGPT.app/"}},
		{Name: "chrome", ProcessNames: []string{"chrome.exe", "msedge.exe", "firefox.exe", "Google Chrome", "Microsoft Edge", "Firefox"}},
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
	// Preserve the legacy opt-out when loading a config written before the
	// usage report was replaced by the smaller local trace file.
	var compatibility struct {
		LocalTraceEnabled      *bool `json:"local_trace_enabled"`
		LocalUsageStatsEnabled *bool `json:"local_usage_stats_enabled"`
	}
	if err := json.Unmarshal(data, &compatibility); err != nil {
		return Settings{}, fmt.Errorf("decode config compatibility fields: %w", err)
	}
	if compatibility.LocalTraceEnabled == nil && compatibility.LocalUsageStatsEnabled != nil {
		settings.LocalTraceEnabled = *compatibility.LocalUsageStatsEnabled
	}
	migrateLegacyActionAliases(settings.Bindings)
	migrateLegacyAppProfiles(&settings)
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = SchemaVersion
	}
	return settings, settings.Validate()
}

func migrateLegacyActionAliases(bindings map[string]map[string]string) {
	aliases := map[string]string{
		"chrome_previous_tab": "tab_previous",
		"chrome_next_tab":     "tab_next",
		"chrome_address_bar":  "focus_location",
		"chrome_new_tab":      "tab_new",
	}
	for _, profileBindings := range bindings {
		for gesture, action := range profileBindings {
			if replacement, found := aliases[action]; found {
				profileBindings[gesture] = replacement
			}
		}
	}
}

func migrateLegacyAppProfiles(settings *Settings) {
	current := defaultAppProfiles()
	windows := legacyWindowsAppProfiles()
	crossPlatform := legacyCrossPlatformAppProfiles()
	profiles := make([]core.AppProfile, 0, len(settings.AppProfiles))
	for _, profile := range settings.AppProfiles {
		name := strings.ToLower(profile.Name)
		if name == "codex" || name == "chrome" {
			if profileAppearsIn(profile, windows) || profileAppearsIn(profile, crossPlatform) {
				profiles = append(profiles, namedProfile(current, name))
				continue
			}
		}
		if isRetiredProfile(name) && !hasProfileBindings(settings.Bindings, name) &&
			(profileAppearsIn(profile, windows) || profileAppearsIn(profile, crossPlatform)) {
			continue
		}
		profiles = append(profiles, profile)
	}
	settings.AppProfiles = profiles
}

func isRetiredProfile(profile string) bool {
	retired := map[string]struct{}{
		"assistant": {}, "chat": {}, "document": {}, "jetbrains": {},
		"media": {}, "notes": {}, "raycast": {}, "terminal": {},
		"typeless": {}, "vscode": {},
	}
	_, found := retired[profile]
	return found
}

func hasProfileBindings(bindings map[string]map[string]string, profile string) bool {
	for name, values := range bindings {
		if strings.EqualFold(name, profile) && len(values) > 0 {
			return true
		}
	}
	return false
}

func profileAppearsIn(profile core.AppProfile, candidates []core.AppProfile) bool {
	for _, candidate := range candidates {
		if reflect.DeepEqual(profile, candidate) {
			return true
		}
	}
	return false
}

func namedProfile(profiles []core.AppProfile, name string) core.AppProfile {
	for _, profile := range profiles {
		if strings.EqualFold(profile.Name, name) {
			return profile
		}
	}
	return core.AppProfile{}
}

func legacyWindowsAppProfiles() []core.AppProfile {
	return []core.AppProfile{
		{Name: "codex", ProcessNames: []string{"ChatGPT.exe"}, PathContains: []string{`\OpenAI.Codex_`}},
		{Name: "chrome", ProcessNames: []string{"chrome.exe", "msedge.exe", "firefox.exe"}},
		{Name: "raycast", ProcessNames: []string{"Raycast.exe"}},
		{Name: "typeless", ProcessNames: []string{"Typeless.exe"}},
		{Name: "notes", ProcessNames: []string{"Typora.exe", "Obsidian.exe"}},
		{Name: "vscode", ProcessNames: []string{"Code.exe"}},
		{Name: "jetbrains", ProcessNames: []string{"pycharm64.exe", "idea64.exe", "goland64.exe"}},
		{Name: "chat", ProcessNames: []string{"QQ.exe", "Weixin.exe", "WeChat.exe"}},
		{Name: "assistant", ProcessNames: []string{"Claude.exe", "CherryStudio.exe", "Cherry Studio.exe"}},
		{Name: "media", ProcessNames: []string{"QQMusic.exe", "Spotify.exe", "vlc.exe"}},
		{Name: "document", ProcessNames: []string{"Acrobat.exe", "WINWORD.EXE", "EXCEL.EXE", "POWERPNT.EXE"}},
		{Name: "terminal", ProcessNames: []string{"WindowsTerminal.exe"}},
	}
}

func legacyCrossPlatformAppProfiles() []core.AppProfile {
	return []core.AppProfile{
		{Name: "codex", ProcessNames: []string{"ChatGPT.exe", "ChatGPT", "Codex"}, PathContains: []string{`\OpenAI.Codex_`, "/Codex.app/", "/ChatGPT.app/"}},
		{Name: "chrome", ProcessNames: []string{"chrome.exe", "msedge.exe", "firefox.exe", "Google Chrome", "Microsoft Edge", "Firefox"}},
		{Name: "raycast", ProcessNames: []string{"Raycast.exe", "Raycast"}},
		{Name: "typeless", ProcessNames: []string{"Typeless.exe", "Typeless"}},
		{Name: "notes", ProcessNames: []string{"Typora.exe", "Obsidian.exe", "Typora", "Obsidian"}},
		{Name: "vscode", ProcessNames: []string{"Code.exe", "Code"}},
		{Name: "jetbrains", ProcessNames: []string{"pycharm64.exe", "idea64.exe", "goland64.exe", "pycharm", "idea", "goland"}},
		{Name: "chat", ProcessNames: []string{"QQ.exe", "Weixin.exe", "WeChat.exe", "QQ", "WeChat"}},
		{Name: "assistant", ProcessNames: []string{"Claude.exe", "CherryStudio.exe", "Cherry Studio.exe", "Claude", "Cherry Studio"}},
		{Name: "media", ProcessNames: []string{"QQMusic.exe", "Spotify.exe", "vlc.exe", "QQMusic", "Spotify", "VLC"}},
		{Name: "document", ProcessNames: []string{"Acrobat.exe", "WINWORD.EXE", "EXCEL.EXE", "POWERPNT.EXE", "Adobe Acrobat", "Microsoft Word", "Microsoft Excel", "Microsoft PowerPoint"}},
		{Name: "terminal", ProcessNames: []string{"WindowsTerminal.exe", "Terminal", "iTerm2", "Warp", "ghostty"}},
	}
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
	if s.VoiceSubmitTimeoutSeconds < 5 || s.VoiceSubmitTimeoutSeconds > 600 {
		return errors.New("voice_submit_timeout_seconds must be between 5 and 600")
	}
	if s.HapticStrength < 0 || s.HapticStrength > 2 {
		return errors.New("haptic_strength must be between 0 and 2")
	}
	if s.ExitHoldSeconds <= 0 {
		return errors.New("exit_hold_seconds must be positive")
	}
	profileNames := make(map[string]struct{}, len(s.AppProfiles))
	for index, profile := range s.AppProfiles {
		if profile.Name == "" {
			return fmt.Errorf("app_profiles[%d].name cannot be empty", index)
		}
		name := strings.ToLower(profile.Name)
		if _, found := profileNames[name]; found {
			return fmt.Errorf("duplicate app profile %q", profile.Name)
		}
		profileNames[name] = struct{}{}
		if len(profile.ProcessNames) == 0 && len(profile.PathContains) == 0 {
			return fmt.Errorf("app profile %q needs process_names or path_contains", profile.Name)
		}
		for _, processName := range profile.ProcessNames {
			if strings.TrimSpace(processName) == "" {
				return fmt.Errorf("app profile %q contains an empty process name", profile.Name)
			}
		}
		for _, fragment := range profile.PathContains {
			if strings.TrimSpace(fragment) == "" {
				return fmt.Errorf("app profile %q contains an empty path fragment", profile.Name)
			}
		}
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
