package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLoadErrorsIdentifyConfigFile(t *testing.T) {
	for _, test := range []struct {
		name, data, detail string
	}{
		{"syntax", `{"poll_hz":`, "decode config"},
		{"type", `{"poll_hz":"fast"}`, "decode config"},
		{"validation", `{"poll_hz":0}`, "poll_hz must be between"},
		{"binding", `{"bindings":{"default":{"a":"typo"}}}`, "unknown action"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "配置 settings.json")
			if err := os.WriteFile(path, []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatal("invalid config was accepted")
			}
			if !strings.HasPrefix(err.Error(), "load config "+strconv.Quote(path)+": ") || !strings.Contains(err.Error(), test.detail) {
				t.Errorf("error must identify the config file and cause, got %v", err)
			}
			switch test.name {
			case "syntax":
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Errorf("syntax error identity was lost: %v", err)
				}
			case "type":
				var typeErr *json.UnmarshalTypeError
				if !errors.As(err, &typeErr) || typeErr.Field != "poll_hz" {
					t.Errorf("type error details were lost: %v", err)
				}
			}
			stored, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(stored) != test.data {
				t.Fatal("loading an invalid config changed its contents")
			}
		})
	}
}

func TestLoadReadErrorRetainsPathError(t *testing.T) {
	path := t.TempDir() // Reading a directory as config must fail on all platforms.
	_, err := Load(path)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != path {
		t.Fatalf("filesystem error identity was lost: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "load config "+strconv.Quote(path)+": ") {
		t.Fatalf("expected consistent config path context: %v", err)
	}
}

func TestLoadStillCreatesMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	settings, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.PollHz != Default().PollHz {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing config was not created: %v", err)
	}
}
