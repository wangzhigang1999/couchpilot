package winplatform

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/desktop"
)

type postedKeyEvent struct {
	key  uint16
	down bool
}

func TestTapHotkeyPostsAndReleasesModifiersInOrder(t *testing.T) {
	var events []postedKeyEvent
	err := tapHotkeyWith(func(key uint16, down bool) error {
		events = append(events, postedKeyEvent{key: key, down: down})
		return nil
	}, func(time.Duration) {}, vkControl, vkShift, vkTab)
	if err != nil {
		t.Fatal(err)
	}
	want := []postedKeyEvent{
		{key: vkControl, down: true},
		{key: vkShift, down: true},
		{key: vkTab, down: true},
		{key: vkTab, down: false},
		{key: vkShift, down: false},
		{key: vkControl, down: false},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%+v want %+v", events, want)
	}
}

func TestTapHotkeyReleasesFirstModifierWhenSecondKeyDownFails(t *testing.T) {
	var events []postedKeyEvent
	dispatchErr := errors.New("shift down failed")
	err := tapHotkeyWith(func(key uint16, down bool) error {
		events = append(events, postedKeyEvent{key: key, down: down})
		if key == vkShift && down {
			return dispatchErr
		}
		return nil
	}, func(time.Duration) {}, vkControl, vkShift, vkTab)
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("error=%v want %v", err, dispatchErr)
	}
	wantLast := postedKeyEvent{key: vkControl, down: false}
	if len(events) == 0 || events[len(events)-1] != wantLast {
		t.Fatalf("first modifier was not released: %+v", events)
	}
}

func TestProcessNameFromPathReturnsOnlyExecutableBaseName(t *testing.T) {
	for path, want := range map[string]string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`: "chrome.exe",
		`C:/Program Files/OpenAI/ChatGPT.exe`:                   "ChatGPT.exe",
		`explorer.exe`:                                          "explorer.exe",
	} {
		if got := processNameFromPath(path); got != want {
			t.Fatalf("processNameFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLogicalPrimaryModifierUsesControlOnWindows(t *testing.T) {
	got, err := windowsModifier(desktop.ModifierPrimary)
	if err != nil {
		t.Fatal(err)
	}
	if got != vkControl {
		t.Fatalf("primary=%#x want control=%#x", got, vkControl)
	}
}

func TestLogicalKeysUseWindowsVirtualKeys(t *testing.T) {
	tests := map[desktop.Key]uint16{
		desktop.KeyEscape: vkEscape, desktop.KeyArrowUp: vkUp,
		desktop.KeyArrowDown: vkDown, desktop.KeyArrowLeft: vkLeft,
		desktop.KeyArrowRight: vkRight, desktop.KeyBackspace: vkBackspace,
		desktop.KeyEnter: vkEnter, desktop.KeyTab: vkTab,
		desktop.KeyPageUp: vkPageUp, desktop.KeyPageDown: vkPageDown,
		desktop.KeyLeftBracket: vkOEM4, desktop.KeyRightBracket: vkOEM6,
		desktop.KeyGrave: vkOEM3, desktop.KeyF: 'F', desktop.KeyK: 'K',
		desktop.KeyL: 'L', desktop.KeyN: 'N', desktop.KeyP: 'P', desktop.KeyT: 'T',
	}
	for key, want := range tests {
		got, err := windowsKey(key)
		if err != nil {
			t.Fatalf("windowsKey(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("windowsKey(%q)=%#x want %#x", key, got, want)
		}
	}
}

func TestPlatformDefaultVoiceKeyRemainsRightAltOnWindows(t *testing.T) {
	key, err := virtualKey("platform_default")
	if err != nil {
		t.Fatal(err)
	}
	if key != vkRightAlt {
		t.Fatalf("platform_default=%#x want %#x", key, vkRightAlt)
	}
}
