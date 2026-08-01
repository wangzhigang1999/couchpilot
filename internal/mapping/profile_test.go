package mapping

import (
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestProfileMatcherUsesPortableApplicationIdentity(t *testing.T) {
	profiles := []core.AppProfile{
		{
			Name:         "codex-store",
			ProcessNames: []string{"ChatGPT.exe"},
			PathContains: []string{`\OpenAI.Codex_`},
		},
		{
			Name:         "codex",
			ProcessNames: []string{"ChatGPT", "Codex"},
			PathContains: []string{"/ChatGPT.app/", "/Codex.app/"},
		},
		{
			Name:         "browser",
			ProcessNames: []string{"chrome.exe", "Google Chrome"},
		},
		{
			Name:         "path-only",
			PathContains: []string{"/special/tools/"},
		},
	}
	matcher := NewProfileMatcher(profiles)

	tests := []struct {
		name     string
		identity core.AppIdentity
		want     string
	}{
		{
			"Windows process and path",
			core.AppIdentity{ProcessName: "CHATGPT.EXE", ExecutablePath: `C:\Program Files\WindowsApps\OpenAI.Codex_1.2.3\ChatGPT.exe`},
			"codex-store",
		},
		{
			"macOS process and path",
			core.AppIdentity{ProcessName: "ChatGPT", ExecutablePath: "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"},
			"codex",
		},
		{
			"exe suffix is portable",
			core.AppIdentity{ProcessName: "chrome", ExecutablePath: "/opt/chrome"},
			"browser",
		},
		{
			"path separators are portable",
			core.AppIdentity{ProcessName: "helper", ExecutablePath: `C:\special\tools\helper.exe`},
			"path-only",
		},
		{
			"unknown application",
			core.AppIdentity{ProcessName: "notes.exe", ExecutablePath: `C:\Notes\notes.exe`},
			"default",
		},
		{
			"empty identity",
			core.AppIdentity{},
			"default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matcher.Match(test.identity); got != test.want {
				t.Fatalf("Match(%+v) = %q, want %q", test.identity, got, test.want)
			}
		})
	}
}

func TestProfileMatcherUsesFirstMatch(t *testing.T) {
	matcher := NewProfileMatcher([]core.AppProfile{
		{Name: "specific", ProcessNames: []string{"app.exe"}, PathContains: []string{"/special/"}},
		{Name: "general", ProcessNames: []string{"app.exe"}},
	})

	identity := core.AppIdentity{
		ProcessName:    "app.exe",
		ExecutablePath: `C:\special\app.exe`,
	}
	if got := matcher.Match(identity); got != "specific" {
		t.Fatalf("Match() = %q, want first matching profile", got)
	}
}
