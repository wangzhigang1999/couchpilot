package engine

import (
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type fakeGamepad struct{}

func (fakeGamepad) Devices() ([]core.DeviceID, error) { return []core.DeviceID{"test:0"}, nil }
func (fakeGamepad) Read(core.DeviceID, float64) (core.State, bool, error) {
	return core.State{}, true, nil
}
func (fakeGamepad) Rumble(core.DeviceID, uint16, uint16) error { return nil }

type fakeDesktop struct {
	profile string
	actions []core.Action
	moves   [][2]int
}

func (f *fakeDesktop) MovePointer(x, y int) error {
	f.moves = append(f.moves, [2]int{x, y})
	return nil
}
func (f *fakeDesktop) Scroll(int) error { return nil }
func (f *fakeDesktop) Perform(action core.Action) error {
	f.actions = append(f.actions, action)
	return nil
}
func (f *fakeDesktop) ForegroundProfile() string { return f.profile }

func TestLTShouldersOverrideChromeTabs(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	if err := engine.Step(core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	if len(desktop.actions) != 1 || desktop.actions[0] != core.WindowCycleNext {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
}

func TestLTShouldersCycleMultipleWindowsUntilLTIsReleased(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{
		{Buttons: core.RightShoulder, LeftTrigger: 1},
		{LeftTrigger: 1},
		{Buttons: core.RightShoulder, LeftTrigger: 1},
		{LeftTrigger: 1},
		{Buttons: core.LeftShoulder, LeftTrigger: 1},
		{},
	}
	for index, state := range states {
		if err := engine.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{
		core.WindowCycleNext,
		core.WindowCycleNext,
		core.WindowCyclePrevious,
		core.WindowCycleCommit,
	}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestDisconnectCommitsWindowSwitch(t *testing.T) {
	desktop := &fakeDesktop{profile: "default"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	if err := engine.Step(core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	engine.disconnect()
	if got := desktop.actions[len(desktop.actions)-1]; got != core.WindowCycleCommit {
		t.Fatalf("expected commit on disconnect, got %v", desktop.actions)
	}
}

func TestSingleChromeShoulderKeepsTabMapping(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	if err := engine.Step(core.State{Buttons: core.RightShoulder}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(desktop.actions) != 1 || desktop.actions[0] != core.TabNext {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
}

func TestHighFrequencyAppBindings(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		button  core.Button
		want    core.Action
	}{
		{"Raycast confirms selection", "raycast", core.A, core.Enter},
		{"Raycast moves selection", "raycast", core.RightShoulder, core.ArrowDown},
		{"Notes find", "notes", core.LeftThumb, core.Find},
		{"VS Code command palette", "vscode", core.LeftThumb, core.CommandPalette},
		{"Chat dismisses overlay", "chat", core.B, core.Escape},
		{"Media toggles playback", "media", core.RightThumb, core.MediaPlayPause},
		{"Documents page down", "document", core.RightShoulder, core.PageDown},
		{"Terminal opens tab", "terminal", core.RightThumb, core.TabNew},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			desktop := &fakeDesktop{profile: test.profile}
			engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
			if err := engine.Step(core.State{Buttons: test.button}, 1.0/120, time.Now()); err != nil {
				t.Fatal(err)
			}
			if len(desktop.actions) != 1 || desktop.actions[0] != test.want {
				t.Fatalf("unexpected actions: %v", desktop.actions)
			}
		})
	}
}

func TestAHoldsLeftMouseUntilReleased(t *testing.T) {
	desktop := &fakeDesktop{profile: "default"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{
		{Buttons: core.A},
		{Buttons: core.A, LeftX: 1},
		{},
	}
	for index, state := range states {
		if err := engine.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.MouseLeftDown, core.MouseLeftUp}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestCodexXUsesRightMouseInsteadOfEscape(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	if err := engine.Step(core.State{Buttons: core.X}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	if err := engine.Step(core.State{}, 1.0/120, now.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	want := []core.Action{core.MouseRightDown, core.MouseRightUp}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestDisconnectReleasesHeldMouseButton(t *testing.T) {
	desktop := &fakeDesktop{profile: "default"}
	engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	if err := engine.Step(core.State{Buttons: core.A}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	engine.disconnect()
	if got := desktop.actions[len(desktop.actions)-1]; got != core.MouseLeftUp {
		t.Fatalf("expected mouse release on disconnect, got %v", desktop.actions)
	}
}

func TestCustomBindingOverridesBuiltIn(t *testing.T) {
	settings := config.Default()
	settings.Bindings = map[string]map[string]string{"default": {"a": string(core.Escape)}}
	desktop := &fakeDesktop{profile: "default"}
	engine := New(settings, fakeGamepad{}, desktop, false, nil)
	if err := engine.Step(core.State{Buttons: core.A}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(desktop.actions) != 1 || desktop.actions[0] != core.Escape {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
}

func TestPrecisionAndBoostPointerSpeed(t *testing.T) {
	distance := func(lt, rt float64) int {
		desktop := &fakeDesktop{}
		engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
		_ = engine.Step(core.State{LeftX: 1, LeftTrigger: lt, RightTrigger: rt}, 1.0/120, time.Now())
		return desktop.moves[0][0]
	}
	if precision, normal, boost := distance(1, 0), distance(0, 0), distance(0, 1); !(precision < normal && normal < boost) {
		t.Fatalf("expected precision < normal < boost, got %d %d %d", precision, normal, boost)
	}
}
