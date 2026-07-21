package winplatform

import (
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

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
