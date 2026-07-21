//go:build darwin

package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDarwinAppLaunchUsesApplicationSupport(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "tester")
	executable := filepath.Join(string(filepath.Separator), "Applications", "CouchPilot.app", "Contents", "MacOS", "CouchPilot")
	want := []string{"run", "--config", filepath.Join(home, "Library", "Application Support", "CouchPilot", "config.json"), "--app-launch"}
	if got := darwinLaunchArgs(nil, executable, home); !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%v want %v", got, want)
	}
}

func TestDarwinCLIArgumentsRemainPortable(t *testing.T) {
	arguments := []string{"doctor", "--config", "config.json"}
	if got := darwinLaunchArgs(arguments, "/Applications/CouchPilot.app/Contents/MacOS/CouchPilot", "/Users/tester"); !reflect.DeepEqual(got, arguments) {
		t.Fatalf("args=%v want %v", got, arguments)
	}
}
