//go:build darwin

package macplatform

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestMatchesMacProfiles(t *testing.T) {
	profiles := []core.AppProfile{
		{Name: "codex", ProcessNames: []string{"Codex", "ChatGPT.exe"}, PathContains: []string{"Codex.app", "ChatGPT.app", "OpenAI.Codex_"}},
		{Name: "chrome", ProcessNames: []string{"Google Chrome", "chrome.exe"}},
	}
	tests := []struct{ path, want string }{
		{"/Applications/Codex.app/Contents/MacOS/Codex", "codex"},
		{"/Applications/ChatGPT.app/Contents/MacOS/ChatGPT", "codex"},
		{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "chrome"},
		{"/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder", "default"},
	}
	for _, test := range tests {
		if got := matchProfile(test.path, profiles); got != test.want {
			t.Errorf("matchProfile(%q)=%q want %q", test.path, got, test.want)
		}
	}
}

func TestFlagsForModifiers(t *testing.T) {
	if got := flagsForModifiers([]uint16{keyCommand, keyShift}); got != flagCommand|flagShift {
		t.Fatalf("flags=%#x", got)
	}
	if got := flagsForModifiers([]uint16{keyControl, keyShift}); got != flagControl|flagShift {
		t.Fatalf("control+shift flags=%#x", got)
	}
	if got := flagsForModifiers([]uint16{keyRightOption}); got != flagOption {
		t.Fatalf("right option flags=%#x", got)
	}
}

func TestTabAndWindowSwitchUseDifferentModifiers(t *testing.T) {
	tabFlags := flagsForModifiers([]uint16{keyControl})
	windowFlags := flagsForModifiers([]uint16{keyCommand})
	if tabFlags != flagControl || windowFlags != flagCommand || tabFlags == windowFlags {
		t.Fatalf("tab flags=%#x window flags=%#x", tabFlags, windowFlags)
	}
}

type postedKeyEvent struct {
	key   uint16
	down  bool
	flags uint64
}

func TestTapHotkeyPressesAndReleasesControl(t *testing.T) {
	var events []postedKeyEvent
	err := tapHotkeyWith(func(key uint16, down bool, flags uint64) error {
		events = append(events, postedKeyEvent{key: key, down: down, flags: flags})
		return nil
	}, func(time.Duration) {}, keyControl, keyTab)
	if err != nil {
		t.Fatal(err)
	}
	want := []postedKeyEvent{
		{key: keyControl, down: true, flags: flagControl},
		{key: keyTab, down: true, flags: flagControl},
		{key: keyTab, down: false, flags: flagControl},
		{key: keyControl, down: false, flags: 0},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%+v want %+v", events, want)
	}
}

func TestTapHotkeyReleasesMultipleModifiersInReverseOrder(t *testing.T) {
	var events []postedKeyEvent
	err := tapHotkeyWith(func(key uint16, down bool, flags uint64) error {
		events = append(events, postedKeyEvent{key: key, down: down, flags: flags})
		return nil
	}, func(time.Duration) {}, keyControl, keyShift, keyTab)
	if err != nil {
		t.Fatal(err)
	}
	want := []postedKeyEvent{
		{key: keyControl, down: true, flags: flagControl},
		{key: keyShift, down: true, flags: flagControl | flagShift},
		{key: keyTab, down: true, flags: flagControl | flagShift},
		{key: keyTab, down: false, flags: flagControl | flagShift},
		{key: keyShift, down: false, flags: flagControl},
		{key: keyControl, down: false, flags: 0},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%+v want %+v", events, want)
	}
}

func TestTapHotkeyReleasesModifiersAfterDispatchFailure(t *testing.T) {
	var events []postedKeyEvent
	dispatchErr := errors.New("dispatch failed")
	err := tapHotkeyWith(func(key uint16, down bool, flags uint64) error {
		events = append(events, postedKeyEvent{key: key, down: down, flags: flags})
		if key == keyTab && down {
			return dispatchErr
		}
		return nil
	}, func(time.Duration) {}, keyControl, keyTab)
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("error=%v want %v", err, dispatchErr)
	}
	wantLast := postedKeyEvent{key: keyControl, down: false, flags: 0}
	if len(events) == 0 || events[len(events)-1] != wantLast {
		t.Fatalf("modifier was not released after failure: %+v", events)
	}
}

func TestTapHotkeyDoesNotLatchModifierWhoseKeyDownFailed(t *testing.T) {
	var events []postedKeyEvent
	dispatchErr := errors.New("shift down failed")
	err := tapHotkeyWith(func(key uint16, down bool, flags uint64) error {
		events = append(events, postedKeyEvent{key: key, down: down, flags: flags})
		if key == keyShift && down {
			return dispatchErr
		}
		return nil
	}, func(time.Duration) {}, keyControl, keyShift, keyTab)
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("error=%v want %v", err, dispatchErr)
	}
	wantLast := postedKeyEvent{key: keyControl, down: false, flags: 0}
	if len(events) == 0 || events[len(events)-1] != wantLast {
		t.Fatalf("failed modifier leaked into cleanup flags: %+v", events)
	}
}

func TestMacVoiceKeyUsesFunctionAsPlatformDefault(t *testing.T) {
	for _, name := range []string{"platform_default", "fn", "function"} {
		binding, err := macVoiceKey(name)
		if err != nil {
			t.Fatalf("macVoiceKey(%q): %v", name, err)
		}
		if binding.key != keyFunction || binding.downFlags != flagSecondaryFn {
			t.Fatalf("macVoiceKey(%q)=%+v", name, binding)
		}
		if got := voiceKeyFlags(binding, true); got != flagSecondaryFn {
			t.Fatalf("Fn key-down flags=%#x", got)
		}
		if got := voiceKeyFlags(binding, false); got != 0 {
			t.Fatalf("Fn key-up flags=%#x", got)
		}
	}
}

func TestMacVoiceKeyKeepsExplicitOptionBindings(t *testing.T) {
	binding, err := macVoiceKey("right_alt")
	if err != nil {
		t.Fatal(err)
	}
	if binding.key != keyRightOption || binding.downFlags != flagOption {
		t.Fatalf("right_alt binding=%+v", binding)
	}
}

func TestScrollLinesInvertWindowsWheelDirection(t *testing.T) {
	tests := []struct{ amount, want int }{
		{120, -1}, {-120, 1}, {240, -2}, {1, -1}, {-1, 1}, {0, 0},
	}
	for _, test := range tests {
		if got := scrollLines(test.amount); got != test.want {
			t.Errorf("scrollLines(%d)=%d want %d", test.amount, got, test.want)
		}
	}
}
