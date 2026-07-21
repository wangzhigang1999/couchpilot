//go:build darwin

package daemon

import (
	"strings"
	"testing"
)

func TestLaunchAgentPlistEscapesPaths(t *testing.T) {
	content, err := launchAgentPlist("/Applications/A&B/couchpilot", "/tmp/a&b/config.json", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"A&amp;B", "a&amp;b", "--verbose", launchAgentLabel} {
		if !strings.Contains(content, expected) {
			t.Fatalf("plist missing %q", expected)
		}
	}
}
