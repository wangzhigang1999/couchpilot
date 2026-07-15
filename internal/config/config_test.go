package config

import (
	"os"
	"path/filepath"
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
	if settings.ControllerIndex != 1 || settings.PollHz != 120 || settings.VoiceKey != "right_alt" || len(settings.AppProfiles) == 0 {
		t.Fatalf("unexpected settings: %+v", settings)
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
