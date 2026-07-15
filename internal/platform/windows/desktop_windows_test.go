package winplatform

import "testing"

func TestDetectsProfiles(t *testing.T) {
	if !isCodexProcessPath(`C:\Program Files\WindowsApps\OpenAI.Codex_1.0\app\ChatGPT.exe`) {
		t.Fatal("expected Codex path")
	}
	if isCodexProcessPath(`C:\Program Files\ChatGPT\ChatGPT.exe`) {
		t.Fatal("generic ChatGPT must not match Codex")
	}
	if !isChromeProcessPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`) {
		t.Fatal("expected Chrome path")
	}
}
