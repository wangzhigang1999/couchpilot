package desktop

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type recordingDriver struct {
	operations []string
	identity   core.AppIdentity
}

func (d *recordingDriver) MovePointer(dx, dy int) error {
	d.operations = append(d.operations, fmt.Sprintf("move:%d,%d", dx, dy))
	return nil
}
func (d *recordingDriver) Scroll(amount int) error {
	d.operations = append(d.operations, fmt.Sprintf("scroll:%d", amount))
	return nil
}
func (d *recordingDriver) TapKey(key Key) error {
	d.operations = append(d.operations, "key:"+string(key))
	return nil
}
func (d *recordingDriver) TapChord(modifiers []Modifier, key Key) error {
	d.operations = append(d.operations, fmt.Sprintf("chord:%v+%s", modifiers, key))
	return nil
}
func (d *recordingDriver) Mouse(button MouseButton, state ButtonState) error {
	d.operations = append(d.operations, fmt.Sprintf("mouse:%s:%s", button, state))
	return nil
}
func (d *recordingDriver) NavigateBack() error {
	d.operations = append(d.operations, "navigate_back")
	return nil
}
func (d *recordingDriver) SwitchWindow(direction Direction) error {
	d.operations = append(d.operations, "window:"+string(direction))
	return nil
}
func (d *recordingDriver) CycleWindow(direction Direction) error {
	d.operations = append(d.operations, "window_cycle:"+string(direction))
	return nil
}
func (d *recordingDriver) CommitWindowSwitch() error {
	d.operations = append(d.operations, "window_commit")
	return nil
}
func (d *recordingDriver) Voice(event VoiceEvent) error {
	d.operations = append(d.operations, "voice:"+string(event))
	return nil
}
func (d *recordingDriver) Media(key MediaKey) error {
	d.operations = append(d.operations, "media:"+string(key))
	return nil
}
func (d *recordingDriver) ForegroundApplication() core.AppIdentity {
	return d.identity
}

func TestExecutorOwnsPortableActionRecipes(t *testing.T) {
	tests := []struct {
		action core.Action
		want   []string
	}{
		{core.ClickLeft, []string{"mouse:left:down", "mouse:left:up"}},
		{core.ClickRight, []string{"mouse:right:down", "mouse:right:up"}},
		{core.NavigateBack, []string{"navigate_back"}},
		{core.Escape, []string{"key:escape"}},
		{core.ArrowUp, []string{"key:arrow_up"}},
		{core.ArrowDown, []string{"key:arrow_down"}},
		{core.ArrowLeft, []string{"key:arrow_left"}},
		{core.ArrowRight, []string{"key:arrow_right"}},
		{core.Backspace, []string{"key:backspace"}},
		{core.Enter, []string{"key:enter"}},
		{core.TabPrevious, []string{"chord:[control shift]+tab"}},
		{core.TabNext, []string{"chord:[control]+tab"}},
		{core.TabNew, []string{"chord:[primary]+t"}},
		{core.FocusLocation, []string{"chord:[primary]+l"}},
		{core.Find, []string{"chord:[primary]+f"}},
		{core.NewDocument, []string{"chord:[primary]+n"}},
		{core.PageUp, []string{"key:page_up"}},
		{core.PageDown, []string{"key:page_down"}},
		{core.CommandPalette, []string{"chord:[primary shift]+p"}},
		{core.QuickOpen, []string{"chord:[primary]+p"}},
		{core.MediaPreviousTrack, []string{"media:previous"}},
		{core.MediaNextTrack, []string{"media:next"}},
		{core.MediaPlayPause, []string{"media:play_pause"}},
		{core.VolumeMute, []string{"media:mute"}},
		{core.WindowPrevious, []string{"window:previous"}},
		{core.WindowNext, []string{"window:next"}},
		{core.CodexBack, []string{"chord:[primary]+left_bracket"}},
		{core.CodexPreviousTask, []string{"chord:[primary shift]+left_bracket"}},
		{core.CodexNextTask, []string{"chord:[primary shift]+right_bracket"}},
		{core.CodexCommandMenu, []string{"chord:[primary]+k"}},
		{core.CodexTerminal, []string{"chord:[primary]+grave"}},
	}

	for _, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			driver := &recordingDriver{}
			executor := NewExecutor(driver, nil)
			if err := executor.Perform(test.action); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(driver.operations, test.want) {
				t.Fatalf("operations = %v, want %v", driver.operations, test.want)
			}
		})
	}
}

func TestExecutorOwnsInternalOperationRecipes(t *testing.T) {
	tests := []struct {
		operation core.Operation
		want      []string
	}{
		{core.MouseLeftDown, []string{"mouse:left:down"}},
		{core.MouseLeftUp, []string{"mouse:left:up"}},
		{core.MouseRightDown, []string{"mouse:right:down"}},
		{core.MouseRightUp, []string{"mouse:right:up"}},
		{core.VoiceTap, []string{"voice:tap"}},
		{core.VoiceDown, []string{"voice:down"}},
		{core.VoiceUp, []string{"voice:up"}},
		{core.WindowCyclePrevious, []string{"window_cycle:previous"}},
		{core.WindowCycleNext, []string{"window_cycle:next"}},
		{core.WindowCycleCommit, []string{"window_commit"}},
	}

	for _, test := range tests {
		t.Run(string(test.operation), func(t *testing.T) {
			driver := &recordingDriver{}
			if err := NewExecutor(driver, nil).PerformOperation(test.operation); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(driver.operations, test.want) {
				t.Fatalf("operations = %v, want %v", driver.operations, test.want)
			}
		})
	}
}

func TestExecutorMatchesForegroundProfileOutsidePlatformAdapter(t *testing.T) {
	driver := &recordingDriver{identity: core.AppIdentity{
		ProcessName:    "ChatGPT.exe",
		ExecutablePath: `C:\Program Files\WindowsApps\OpenAI.Codex_1.2.3\ChatGPT.exe`,
	}}
	executor := NewExecutor(driver, []core.AppProfile{{
		Name:         "codex",
		ProcessNames: []string{"ChatGPT"},
		PathContains: []string{"/OpenAI.Codex_"},
	}})

	profile, processName := executor.ForegroundContext()
	if profile != "codex" || processName != "ChatGPT.exe" {
		t.Fatalf("ForegroundContext() = %q, %q", profile, processName)
	}
}

func TestExecutorRejectsUnknownAndUnexpandedActions(t *testing.T) {
	driver := &recordingDriver{}
	executor := NewExecutor(driver, nil)

	for _, action := range []core.Action{core.Voice, "not_real"} {
		if err := executor.Perform(action); err == nil {
			t.Fatalf("Perform(%q) unexpectedly succeeded", action)
		}
	}
	if err := executor.PerformOperation("not_real"); err == nil {
		t.Fatal("unknown operation unexpectedly succeeded")
	}
}

func TestExecutorSupportsEveryPublicActionAfterEngineExpansion(t *testing.T) {
	for _, action := range core.KnownActions {
		if action == core.Voice {
			continue
		}
		driver := &recordingDriver{}
		if err := NewExecutor(driver, nil).Perform(action); err != nil {
			t.Errorf("public action %q is missing an executor recipe: %v", action, err)
		}
	}
}
