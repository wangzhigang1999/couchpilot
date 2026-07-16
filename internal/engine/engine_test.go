package engine

import (
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type fakeGamepad struct {
	rumbles *[][2]uint16
}

func (fakeGamepad) Devices() ([]core.DeviceID, error) { return []core.DeviceID{"test:0"}, nil }
func (fakeGamepad) Read(core.DeviceID, float64) (core.State, bool, error) {
	return core.State{}, true, nil
}
func (f fakeGamepad) Rumble(_ core.DeviceID, left, right uint16) error {
	if f.rumbles != nil {
		*f.rumbles = append(*f.rumbles, [2]uint16{left, right})
	}
	return nil
}

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

func TestCodexVoiceThenASubmitsWithoutClicking(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{
		{Buttons: core.Y},
		{},
		{Buttons: core.A},
		{},
	}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.VoiceTap, core.Enter}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestCodexBDeletesAndKeepsVoiceSubmitArmed(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{
		{Buttons: core.Y},
		{},
		{Buttons: core.B},
		{},
		{Buttons: core.A},
		{},
	}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.VoiceTap, core.Backspace, core.Enter}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestHoldingCodexBRepeatsBackspaceUntilReleased(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []struct {
		state core.State
		at    time.Time
	}{
		{core.State{Buttons: core.Y}, now},
		{core.State{}, now.Add(time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(2 * time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(300 * time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(330 * time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(410 * time.Millisecond)},
		{core.State{}, now.Add(500 * time.Millisecond)},
	}
	for _, item := range states {
		if err := controller.Step(item.state, 1.0/120, item.at); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.VoiceTap, core.Backspace, core.Backspace, core.Backspace}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestCodexBOutsideVoiceComposeStillNavigatesBack(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{Buttons: core.B}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(desktop.actions) != 1 || desktop.actions[0] != core.CodexBack {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
}

func TestPointerMovementCancelsCodexVoiceSubmit(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{
		{Buttons: core.Y},
		{},
		{LeftX: 1},
		{Buttons: core.A},
		{},
	}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.VoiceTap, core.MouseLeftDown, core.MouseLeftUp}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestCodexRTAAlwaysSubmits(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{Buttons: core.A, RightTrigger: 1}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(desktop.actions) != 1 || desktop.actions[0] != core.Enter {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
}

func TestVoiceEditWhitelistExcludesBrowser(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{{Buttons: core.Y}, {}, {Buttons: core.A}, {}}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.VoiceTap, core.MouseLeftDown, core.MouseLeftUp}
	if len(desktop.actions) != len(want) {
		t.Fatalf("unexpected actions: %v", desktop.actions)
	}
	for index := range want {
		if desktop.actions[index] != want[index] {
			t.Fatalf("unexpected actions: %v", desktop.actions)
		}
	}
}

func TestVoiceEditWhitelistIncludesChatAndAssistants(t *testing.T) {
	for _, profile := range []string{"chat", "assistant"} {
		t.Run(profile, func(t *testing.T) {
			desktop := &fakeDesktop{profile: profile}
			controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
			now := time.Now()
			states := []core.State{
				{Buttons: core.Y},
				{},
				{Buttons: core.B},
				{},
				{Buttons: core.A},
				{},
			}
			for index, state := range states {
				if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
			}
			want := []core.Action{core.VoiceTap, core.Backspace, core.Enter}
			if len(desktop.actions) != len(want) {
				t.Fatalf("unexpected actions: %v", desktop.actions)
			}
			for index := range want {
				if desktop.actions[index] != want[index] {
					t.Fatalf("unexpected actions: %v", desktop.actions)
				}
			}
		})
	}
}

func TestCodexVoiceSubmitTimesOut(t *testing.T) {
	settings := config.Default()
	settings.VoiceSubmitTimeoutSeconds = 5
	desktop := &fakeDesktop{profile: "codex"}
	controller := New(settings, fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []struct {
		state core.State
		at    time.Time
	}{
		{core.State{Buttons: core.Y}, now},
		{core.State{}, now.Add(time.Millisecond)},
		{core.State{Buttons: core.A}, now.Add(6 * time.Second)},
		{core.State{}, now.Add(6*time.Second + time.Millisecond)},
	}
	for _, item := range states {
		if err := controller.Step(item.state, 1.0/120, item.at); err != nil {
			t.Fatal(err)
		}
	}
	want := []core.Action{core.VoiceTap, core.MouseLeftDown, core.MouseLeftUp}
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

func TestButtonActionProducesPerceptibleHapticPulse(t *testing.T) {
	var rumbles [][2]uint16
	gamepad := fakeGamepad{rumbles: &rumbles}
	controller := New(config.Default(), gamepad, &fakeDesktop{}, false, nil)
	controller.device = "test:0"
	now := time.Now()
	if err := controller.Step(core.State{Buttons: core.A}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	if len(rumbles) != 1 || rumbles[0][0] < 8000 || rumbles[0][1] < 20000 {
		t.Fatalf("expected a perceptible click pulse, got %v", rumbles)
	}
	if err := controller.Step(core.State{}, 1.0/120, now.Add(200*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if got := rumbles[len(rumbles)-1]; got != [2]uint16{} {
		t.Fatalf("expected motors to stop, got %v", rumbles)
	}
}

func TestWindowCommitIsStrongerThanCycleTick(t *testing.T) {
	var rumbles [][2]uint16
	gamepad := fakeGamepad{rumbles: &rumbles}
	controller := New(config.Default(), gamepad, &fakeDesktop{}, false, nil)
	controller.device = "test:0"
	now := time.Now()
	if err := controller.Step(core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	cycle := rumbles[len(rumbles)-1]
	if err := controller.Step(core.State{}, 1.0/120, now.Add(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	commit := rumbles[len(rumbles)-1]
	if commit[0] <= cycle[0] || commit[1] <= cycle[1] {
		t.Fatalf("expected commit %v to be stronger than cycle %v", commit, cycle)
	}
}

func TestHapticsCanBeDisabled(t *testing.T) {
	var rumbles [][2]uint16
	settings := config.Default()
	settings.HapticsEnabled = false
	controller := New(settings, fakeGamepad{rumbles: &rumbles}, &fakeDesktop{}, false, nil)
	controller.device = "test:0"
	if err := controller.Step(core.State{Buttons: core.A}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(rumbles) != 0 {
		t.Fatalf("expected no haptics, got %v", rumbles)
	}
}
