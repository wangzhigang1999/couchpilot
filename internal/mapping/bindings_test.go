package mapping

import (
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestBuiltInBindingsAreAStableUserContract(t *testing.T) {
	tests := []struct {
		profile string
		gesture string
		action  core.Action
		source  string
	}{
		{"default", "a", core.ClickLeft, "default"},
		{"default", "b", core.NavigateBack, "default"},
		{"default", "x", core.ClickRight, "default"},
		{"default", "y", core.Voice, "default"},
		{"default", "voice+a", core.Enter, "default"},
		{"default", "rt+a", core.Enter, "default"},
		{"default", "rt+b", core.Backspace, "default"},
		{"default", "rt+x", core.Escape, "default"},
		{"default", "rt+y", core.Find, "default"},
		{"default", "dpad_up", core.ArrowUp, "default"},
		{"default", "dpad_down", core.ArrowDown, "default"},
		{"default", "dpad_left", core.ArrowLeft, "default"},
		{"default", "dpad_right", core.ArrowRight, "default"},
		{"default", "lt+lb", core.WindowPrevious, "default"},
		{"default", "lt+rb", core.WindowNext, "default"},

		{"codex", "a", core.ClickLeft, "default"},
		{"codex", "b", core.CodexBack, "codex"},
		{"codex", "x", core.ClickRight, "default"},
		{"codex", "y", core.Voice, "default"},
		{"codex", "voice+a", core.Enter, "default"},
		{"codex", "voice+b", core.Backspace, "codex"},
		{"codex", "lb", core.CodexPreviousTask, "codex"},
		{"codex", "rb", core.CodexNextTask, "codex"},
		{"codex", "l3", core.CodexCommandMenu, "codex"},
		{"codex", "r3", core.CodexTerminal, "codex"},
		{"codex", "rt+a", core.Enter, "default"},
		{"codex", "dpad_up", core.ArrowUp, "default"},
		{"codex", "lt+lb", core.WindowPrevious, "default"},
		{"codex", "lt+rb", core.WindowNext, "default"},

		{"chrome", "a", core.ClickLeft, "default"},
		{"chrome", "b", core.NavigateBack, "default"},
		{"chrome", "x", core.ClickRight, "default"},
		{"chrome", "y", core.Voice, "default"},
		{"chrome", "voice+a", core.Enter, "default"},
		{"chrome", "rt+a", core.Enter, "default"},
		{"chrome", "rt+b", core.Backspace, "default"},
		{"chrome", "rt+x", core.Escape, "default"},
		{"chrome", "rt+y", core.Find, "default"},
		{"chrome", "lb", core.TabPrevious, "chrome"},
		{"chrome", "rb", core.TabNext, "chrome"},
		{"chrome", "l3", core.FocusLocation, "chrome"},
		{"chrome", "r3", core.TabNew, "chrome"},
		{"chrome", "dpad_down", core.ArrowDown, "default"},
		{"chrome", "lt+lb", core.WindowPrevious, "default"},
		{"chrome", "lt+rb", core.WindowNext, "default"},
	}

	resolver := NewResolver(nil)
	for _, test := range tests {
		name := test.profile + "/" + test.gesture
		t.Run(name, func(t *testing.T) {
			got := resolver.ResolveDetailed(test.profile, test.gesture)
			if got.Resolution != BindingBound || got.Action != test.action || got.BindingProfile != test.source {
				t.Fatalf("ResolveDetailed(%q, %q) = %+v, want action=%q from %q",
					test.profile, test.gesture, got, test.action, test.source)
			}
		})
	}
}

func TestProfileOverrideCanReplaceDisableAndExtendBuiltIns(t *testing.T) {
	resolver := NewResolver(map[string]map[string]string{
		"chrome": {
			"a":     string(core.Enter),
			"rb":    "",
			"rt+lb": string(core.Find),
		},
		"custom": {
			"x": string(core.Escape),
		},
	})

	tests := []struct {
		name       string
		profile    string
		gesture    string
		action     core.Action
		resolution BindingResolution
		source     string
	}{
		{"replace fallback", "chrome", "a", core.Enter, BindingBound, "chrome"},
		{"disable built-in", "chrome", "rb", "", BindingDisabled, "chrome"},
		{"extend profile", "chrome", "rt+lb", core.Find, BindingBound, "chrome"},
		{"custom action", "custom", "x", core.Escape, BindingBound, "custom"},
		{"custom fallback", "custom", "a", core.ClickLeft, BindingBound, "default"},
		{"unknown gesture", "custom", "back", "", BindingUnbound, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolver.ResolveDetailed(test.profile, test.gesture)
			if got.Action != test.action || got.Resolution != test.resolution || got.BindingProfile != test.source {
				t.Fatalf("ResolveDetailed(%q, %q) = %+v", test.profile, test.gesture, got)
			}
		})
	}
}
