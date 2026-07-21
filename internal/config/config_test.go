package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestLoadPartialConfigAndKeepDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"controller_index":1,"voice_mode":"tap"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ControllerIndex != 1 || settings.PollHz != 120 || settings.VoiceKey != "platform_default" || !settings.LocalTraceEnabled || len(settings.AppProfiles) != 2 {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestLocalTraceCanBeDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	settings := Default()
	settings.LocalTraceEnabled = false
	if err := Save(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LocalTraceEnabled {
		t.Fatal("local trace should remain disabled after a round trip")
	}
}

func TestLegacyUsageOptOutDisablesTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"local_usage_stats_enabled":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.LocalTraceEnabled {
		t.Fatal("legacy local usage opt-out should disable local trace")
	}
}

func TestDefaultProfilesOnlyIncludeCodexAndChrome(t *testing.T) {
	profiles := Default().AppProfiles
	if len(profiles) != 2 || profiles[0].Name != "codex" || profiles[1].Name != "chrome" {
		t.Fatalf("default profiles = %+v", profiles)
	}
}

func TestLoadDropsRetiredUncustomizedBuiltinProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(struct {
		AppProfiles []core.AppProfile `json:"app_profiles"`
	}{AppProfiles: legacyWindowsAppProfiles()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settings.AppProfiles, defaultAppProfiles()) {
		t.Fatalf("migrated profiles = %+v", settings.AppProfiles)
	}
}

func TestLoadPreservesCustomizedRetiredProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"app_profiles":[{"name":"notes","process_names":["Obsidian"]}],"bindings":{"notes":{"b":"escape"}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.AppProfiles) != 1 || settings.AppProfiles[0].Name != "notes" {
		t.Fatalf("customized profiles = %+v", settings.AppProfiles)
	}
}

func TestLoadMigratesChromeActionAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"bindings":{"chrome":{"lb":"chrome_previous_tab","rb":"chrome_next_tab","l3":"chrome_address_bar","r3":"chrome_new_tab"}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"lb": "tab_previous", "rb": "tab_next", "l3": "focus_location", "r3": "tab_new"}
	for gesture, action := range want {
		if settings.Bindings["chrome"][gesture] != action {
			t.Fatalf("%s action = %q, want %q", gesture, settings.Bindings["chrome"][gesture], action)
		}
	}
}

func TestLoadPreservesRestrictedBuiltinMatchers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"app_profiles":[{"name":"codex","process_names":["MyCodex"]},{"name":"chrome","process_names":["my-chrome"]}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.AppProfiles) != 2 || !reflect.DeepEqual(settings.AppProfiles[0].ProcessNames, []string{"MyCodex"}) ||
		!reflect.DeepEqual(settings.AppProfiles[1].ProcessNames, []string{"my-chrome"}) {
		t.Fatalf("restricted profiles changed = %+v", settings.AppProfiles)
	}
}

func TestLoadPreservesLegacyProfilesWithCustomBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(struct {
		AppProfiles []core.AppProfile            `json:"app_profiles"`
		Bindings    map[string]map[string]string `json:"bindings"`
	}{
		AppProfiles: legacyWindowsAppProfiles(),
		Bindings:    map[string]map[string]string{"notes": {"b": "escape"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(defaultAppProfiles(), core.AppProfile{Name: "notes", ProcessNames: []string{"Typora.exe", "Obsidian.exe"}})
	if !reflect.DeepEqual(settings.AppProfiles, want) {
		t.Fatalf("customized legacy profiles changed = %+v", settings.AppProfiles)
	}
}

func TestLoadMigratesStockEntriesAlongsideCustomProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	profiles := append(legacyWindowsAppProfiles(), core.AppProfile{Name: "custom", ProcessNames: []string{"custom.exe"}})
	data, err := json.Marshal(struct {
		AppProfiles []core.AppProfile `json:"app_profiles"`
	}{AppProfiles: profiles})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(defaultAppProfiles(), core.AppProfile{Name: "custom", ProcessNames: []string{"custom.exe"}})
	if !reflect.DeepEqual(settings.AppProfiles, want) {
		t.Fatalf("mixed migrated profiles = %+v", settings.AppProfiles)
	}
}

func TestRejectsInvalidAppProfiles(t *testing.T) {
	tests := []struct {
		name     string
		profiles []core.AppProfile
	}{
		{"empty name", []core.AppProfile{{ProcessNames: []string{"app.exe"}}}},
		{"no matcher", []core.AppProfile{{Name: "app"}}},
		{"duplicate name", []core.AppProfile{{Name: "app", ProcessNames: []string{"a.exe"}}, {Name: "APP", ProcessNames: []string{"b.exe"}}}},
		{"empty process", []core.AppProfile{{Name: "app", ProcessNames: []string{""}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := Default()
			settings.AppProfiles = test.profiles
			if settings.Validate() == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestEmptyAppProfilesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	settings := Default()
	settings.AppProfiles = []core.AppProfile{}
	if err := Save(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AppProfiles == nil || len(loaded.AppProfiles) != 0 {
		t.Fatalf("expected explicit empty app profiles, got %+v", loaded.AppProfiles)
	}
}

func TestRejectsUnknownVoiceMode(t *testing.T) {
	settings := Default()
	settings.VoiceMode = "unknown"
	if settings.Validate() == nil {
		t.Fatal("expected validation error")
	}
}

func TestRejectsOutOfRangeVoiceSubmitTimeout(t *testing.T) {
	settings := Default()
	settings.VoiceSubmitTimeoutSeconds = 4
	if settings.Validate() == nil {
		t.Fatal("expected validation error")
	}
}

func TestRejectsOutOfRangeHapticStrength(t *testing.T) {
	settings := Default()
	settings.HapticStrength = 2.1
	if settings.Validate() == nil {
		t.Fatal("expected validation error")
	}
}
