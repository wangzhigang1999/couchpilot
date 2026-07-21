package winplatform

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
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

func TestMatchesConfiguredProfiles(t *testing.T) {
	profiles := []core.AppProfile{
		{Name: "codex", ProcessNames: []string{"ChatGPT.exe"}, PathContains: []string{`\OpenAI.Codex_`}},
		{Name: "browser", ProcessNames: []string{"chrome.exe", "msedge.exe"}},
		{Name: "notes", ProcessNames: []string{"Typora.exe", "Obsidian.exe"}},
	}
	tests := []struct {
		path string
		want string
	}{
		{`C:\Program Files\WindowsApps\OpenAI.Codex_1.0\app\ChatGPT.exe`, "codex"},
		{`C:\Program Files\ChatGPT\ChatGPT.exe`, "default"},
		{`C:\Program Files\Google\Chrome\Application\CHROME.EXE`, "browser"},
		{`C:/Program Files/Typora/Typora.exe`, "notes"},
		{`C:\Windows\explorer.exe`, "default"},
	}
	for _, test := range tests {
		if got := matchProfile(test.path, profiles); got != test.want {
			t.Errorf("matchProfile(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestFirstMatchingProfileWins(t *testing.T) {
	profiles := []core.AppProfile{
		{Name: "specific", ProcessNames: []string{"app.exe"}, PathContains: []string{"special"}},
		{Name: "generic", ProcessNames: []string{"app.exe"}},
	}
	if got := matchProfile(`C:\special\app.exe`, profiles); got != "specific" {
		t.Fatalf("got %q, want specific", got)
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

func TestPlatformDefaultVoiceKeyRemainsRightAltOnWindows(t *testing.T) {
	key, err := virtualKey("platform_default")
	if err != nil {
		t.Fatal(err)
	}
	if key != vkRightAlt {
		t.Fatalf("platform_default=%#x want %#x", key, vkRightAlt)
	}
}
