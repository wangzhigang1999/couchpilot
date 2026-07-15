package config

import (
	"os"
	"path/filepath"
	"testing"
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
	if settings.ControllerIndex != 1 || settings.PollHz != 120 || settings.VoiceKey != "right_alt" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestRejectsUnknownVoiceMode(t *testing.T) {
	settings := Default()
	settings.VoiceMode = "unknown"
	if settings.Validate() == nil {
		t.Fatal("expected validation error")
	}
}
