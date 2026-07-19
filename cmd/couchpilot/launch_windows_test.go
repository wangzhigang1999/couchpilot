//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathForExecutablePrefersPortableConfig(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	portable := filepath.Join(bin, "config.json")
	parent := filepath.Join(root, "config.json")
	for _, path := range []string{parent, portable} {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if got := configPathForExecutable(filepath.Join(bin, "couchpilot.exe")); got != portable {
		t.Fatalf("config path = %q, want %q", got, portable)
	}
}

func TestConfigPathForExecutableFallsBackToParentThenPortableDefault(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bin, "couchpilot.exe")
	parent := filepath.Join(root, "config.json")
	if err := os.WriteFile(parent, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := configPathForExecutable(executable); got != parent {
		t.Fatalf("parent config path = %q, want %q", got, parent)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	portable := filepath.Join(bin, "config.json")
	if got := configPathForExecutable(executable); got != portable {
		t.Fatalf("new config path = %q, want %q", got, portable)
	}
}
