package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
)

func TestLoadAppProfilesReplaceDefaultsWithoutInheritingFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		data     string
		profiles []core.AppProfile
		identity core.AppIdentity
		match    string
	}{
		{
			"process only",
			`{"app_profiles":[{"name":"editor","process_names":["Editor"]}]}`,
			[]core.AppProfile{{Name: "editor", ProcessNames: []string{"Editor"}}},
			core.AppIdentity{ProcessName: "Editor", ExecutablePath: "/Applications/Editor.app/Contents/MacOS/Editor"}, "editor",
		},
		{
			"path only",
			`{"app_profiles":[{"name":"editor","path_contains":["/Editor.app/"]}]}`,
			[]core.AppProfile{{Name: "editor", PathContains: []string{"/Editor.app/"}}},
			core.AppIdentity{ProcessName: "helper", ExecutablePath: "/Applications/Editor.app/Contents/MacOS/helper"}, "editor",
		},
		{
			"customized builtin",
			`{"app_profiles":[{"name":"codex","process_names":["MyCodex"]}]}`,
			[]core.AppProfile{{Name: "codex", ProcessNames: []string{"MyCodex"}}},
			core.AppIdentity{ProcessName: "MyCodex", ExecutablePath: `C:\Tools\MyCodex.exe`}, "codex",
		},
		{"omitted", `{}`, defaultAppProfiles(), core.AppIdentity{ProcessName: "chrome.exe"}, "chrome"},
		{"empty", `{"app_profiles":[]}`, []core.AppProfile{}, core.AppIdentity{ProcessName: "chrome.exe"}, "default"},
		{"null", `{"app_profiles":null}`, []core.AppProfile{}, core.AppIdentity{ProcessName: "chrome.exe"}, "default"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			settings, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(settings.AppProfiles, test.profiles) {
				t.Errorf("profiles=%+v, want %+v", settings.AppProfiles, test.profiles)
			}
			if got := mapping.NewProfileMatcher(settings.AppProfiles).Match(test.identity); got != test.match {
				t.Errorf("matched %q, want %q", got, test.match)
			}
			if settings.PollHz != Default().PollHz {
				t.Fatal("unrelated defaults changed")
			}
			stored, err := os.ReadFile(path)
			if err != nil || string(stored) != test.data {
				t.Fatalf("source config changed: %q, error=%v", stored, err)
			}
		})
	}
}

func TestLoadIncompleteAppProfileCannotBorrowRequiredDefaultFields(t *testing.T) {
	for _, profile := range []string{`{"name":"custom"}`, `{"process_names":["Editor"]}`, `{}`, `null`} {
		t.Run(profile, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(`{"app_profiles":[`+profile+`]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("incomplete profile was accepted using inherited default fields")
			}
		})
	}
}
