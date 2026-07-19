package daemon

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestStartupTaskXML(t *testing.T) {
	taskXML, err := startupTaskXML(`DESKTOP\Test User`, `C:\Apps & Tools\couchpilot.exe`, `C:\My Files\config.json`, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal([]byte(taskXML), new(any)); err != nil {
		t.Fatalf("invalid task XML: %v", err)
	}
	checks := []string{
		`<LogonTrigger>`,
		`<RestartOnFailure><Interval>PT1M</Interval><Count>10</Count></RestartOnFailure>`,
		`C:\Apps &amp; Tools\couchpilot.exe`,
		`run --config &#34;C:\My Files\config.json&#34;`,
		`--verbose`,
	}
	for _, expected := range checks {
		if !strings.Contains(taskXML, expected) {
			t.Errorf("task XML does not contain %q", expected)
		}
	}
}
