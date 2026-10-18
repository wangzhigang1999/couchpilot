package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadRejectsNullDocument(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
	}{
		{"plain", "null"},
		{"whitespace", " \t\r\nnull\r\n "},
		{"BOM", "\xef\xbb\xbfnull"},
		{"BOM and whitespace", "\xef\xbb\xbf\r\n null \t\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			settings, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), "JSON object") {
				t.Errorf("expected an object-required error, got %v", err)
			}
			if !reflect.DeepEqual(settings, Settings{}) {
				t.Errorf("invalid document returned usable defaults: %+v", settings)
			}
			stored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(stored) != test.data {
				t.Fatal("loading an invalid config rewrote the original file")
			}
		})
	}
}

func TestLoadObjectDocumentKeepsDefaults(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
	}{
		{"empty object", "{}"},
		{"whitespace", " \r\n{}\t\n"},
		{"BOM", "\xef\xbb\xbf{}"},
		{"nullable member", `{"bindings":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			settings, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(settings, Default()) {
				t.Fatalf("object changed defaults: %+v", settings)
			}
		})
	}
}
