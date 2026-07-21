package engine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
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
	profile      string
	processName  string
	actions      []core.Action
	moves        [][2]int
	performError error
	performHook  func(core.Action, int) error
	contextCalls int
}

func (f *fakeDesktop) MovePointer(x, y int) error {
	f.moves = append(f.moves, [2]int{x, y})
	return nil
}
func (f *fakeDesktop) Scroll(int) error { return nil }
func (f *fakeDesktop) Perform(action core.Action) error {
	f.actions = append(f.actions, action)
	if f.performHook != nil {
		if err := f.performHook(action, len(f.actions)); err != nil {
			return err
		}
	}
	return f.performError
}
func (f *fakeDesktop) ForegroundContext() (string, string) {
	f.contextCalls++
	return f.profile, f.processName
}

type fakeTraceRecorder struct {
	observations []trace.Fact
}

func (f *fakeTraceRecorder) Record(observation trace.Fact) {
	f.observations = append(f.observations, observation)
}

func TestResolveDetailedReportsBindingProvenance(t *testing.T) {
	resolver := NewResolver(map[string]map[string]string{
		"chrome": {
			"a": "",
		},
	})
	tests := []struct {
		name           string
		profile        string
		gesture        string
		activeProfile  string
		bindingProfile string
		action         core.Action
		resolution     BindingResolution
	}{
		{"active profile", "chrome", "rb", "chrome", "chrome", core.TabNext, BindingBound},
		{"default fallback", "chrome", "dpad_up", "chrome", "default", core.ArrowUp, BindingBound},
		{"explicitly disabled", "chrome", "a", "chrome", "chrome", "", BindingDisabled},
		{"unbound", "chrome", "start", "chrome", "", "", BindingUnbound},
		{"empty profile normalizes", "", "a", "default", "default", core.ClickLeft, BindingBound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolver.ResolveDetailed(test.profile, test.gesture)
			if got.ActiveProfile != test.activeProfile || got.BindingProfile != test.bindingProfile ||
				got.Gesture != test.gesture || got.Action != test.action || got.Resolution != test.resolution {
				t.Fatalf("resolution = %+v", got)
			}
		})
	}
	if action, found := resolver.Resolve("chrome", "a"); found || action != "" {
		t.Fatalf("Resolve compatibility lost disabled semantics: action=%q found=%t", action, found)
	}
}

type smoothDesktop struct {
	fakeDesktop
	events []smoothScrollEvent
}

type smoothScrollEvent struct {
	amount float64
	phase  core.SmoothScrollPhase
}

func (d *smoothDesktop) ScrollSmooth(amount float64, phase core.SmoothScrollPhase) error {
	d.events = append(d.events, smoothScrollEvent{amount: amount, phase: phase})
	return nil
}

func TestSmoothScrollerReceivesEveryFrame(t *testing.T) {
	desktop := &smoothDesktop{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	if err := controller.Step(core.State{RightY: 0.5}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(desktop.events) != 1 {
		t.Fatalf("smooth scroll calls=%d want 1", len(desktop.events))
	}
	event := desktop.events[0]
	maximum := config.Default().ScrollUnitsPerSecond * 0.5 / 120
	if event.amount <= 0 || event.amount >= maximum {
		t.Fatalf("smoothed amount=%f want between 0 and %f", event.amount, maximum)
	}
	if event.phase != core.SmoothScrollBegan {
		t.Fatalf("phase=%v want began", event.phase)
	}
	for index := 0; index < 120; index++ {
		if err := controller.Step(core.State{}, 1.0/120, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	last := desktop.events[len(desktop.events)-1]
	if last.phase != core.SmoothScrollEnded || last.amount != 0 {
		t.Fatalf("last event=%+v want ended", last)
	}
}

func TestStepReadsForegroundContextAtMostOnce(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome", processName: "Google Chrome"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(&fakeTraceRecorder{})
	state := core.State{Buttons: core.A, LeftTrigger: 1}
	if err := controller.Step(state, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if desktop.contextCalls != 1 {
		t.Fatalf("foreground context reads=%d want 1", desktop.contextCalls)
	}
}

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

func TestReleasedLTDoesNotModifyLaterChromeShoulder(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	now := time.Now()
	states := []core.State{
		{LeftTrigger: 1},
		{},
		{Buttons: core.RightShoulder},
	}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(desktop.actions, []core.Action{core.TabNext}) {
		t.Fatalf("unexpected actions after LT release: %v", desktop.actions)
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
	settings.Bindings = map[string]map[string]string{"default": {"a": string(core.Enter)}}
	desktop := &fakeDesktop{profile: "default"}
	engine := New(settings, fakeGamepad{}, desktop, false, nil)
	if err := engine.Step(core.State{Buttons: core.A}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(desktop.actions) != 1 || desktop.actions[0] != core.Enter {
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

func TestTraceRecordsDefaultFallbackWithStepTimestamp(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	recorder := &fakeTraceRecorder{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Date(2026, 7, 20, 1, 2, 3, 0, time.UTC)

	if err := controller.Step(core.State{Buttons: core.DPadUp}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	if len(recorder.observations) != 1 {
		t.Fatalf("observations = %+v", recorder.observations)
	}
	got := recorder.observations[0]
	if !got.At.Equal(now) || got.ActiveProfile != "chrome" || got.BindingProfile != "default" ||
		got.Control != "dpad_up" || got.Gesture != "dpad_up" || got.Action != string(core.ArrowUp) ||
		got.Resolution != trace.Bound || got.Outcome != trace.Success {
		t.Fatalf("observation = %+v", got)
	}
}

func TestTraceRecordsEachDigitalRisingEdgeOnce(t *testing.T) {
	desktop := &fakeDesktop{profile: "default"}
	recorder := &fakeTraceRecorder{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Now()
	states := []core.State{
		{Buttons: core.A},
		{Buttons: core.A},
		{},
		{},
		{Buttons: core.A},
	}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.observations) != 2 {
		t.Fatalf("held/released button was double counted: %+v", recorder.observations)
	}
	for _, observation := range recorder.observations {
		if observation.Control != "a" || observation.Action != string(core.ClickLeft) || observation.Outcome != trace.Success {
			t.Fatalf("observation = %+v", observation)
		}
	}
}

func TestTraceRecordsDisabledAndUnboundControls(t *testing.T) {
	settings := config.Default()
	settings.Bindings = map[string]map[string]string{"chrome": {"a": ""}}
	desktop := &fakeDesktop{profile: "chrome"}
	recorder := &fakeTraceRecorder{}
	controller := New(settings, fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Now()

	for index, state := range []core.State{{Buttons: core.A}, {}, {Buttons: core.Start}} {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if len(desktop.actions) != 0 || len(recorder.observations) != 2 {
		t.Fatalf("actions=%v observations=%+v", desktop.actions, recorder.observations)
	}
	disabled, unbound := recorder.observations[0], recorder.observations[1]
	if disabled.Control != "a" || disabled.BindingProfile != "chrome" || disabled.Resolution != trace.Disabled || disabled.Outcome != trace.NoOutcome {
		t.Fatalf("disabled observation = %+v", disabled)
	}
	if unbound.Control != "start" || unbound.Gesture != "start" || unbound.BindingProfile != "" || unbound.Resolution != trace.Unbound || unbound.Outcome != trace.NoOutcome {
		t.Fatalf("unbound observation = %+v", unbound)
	}
}

func TestTraceRecordsChordButNotWindowCommit(t *testing.T) {
	desktop := &fakeDesktop{profile: "chrome"}
	recorder := &fakeTraceRecorder{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Now()
	states := []core.State{
		{Buttons: core.RightShoulder, LeftTrigger: 1},
		{Buttons: core.RightShoulder, LeftTrigger: 1},
		{LeftTrigger: 1},
		{},
	}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.observations) != 2 {
		t.Fatalf("window commit or held chord was double counted: %+v", recorder.observations)
	}
	trigger, chord := recorder.observations[0], recorder.observations[1]
	if trigger.Control != "lt" || trigger.Resolution != trace.Observed || trigger.Outcome != trace.NoOutcome {
		t.Fatalf("trigger observation = %+v", trigger)
	}
	if chord.Control != "rb" || chord.Gesture != "lt+rb" || chord.Action != string(core.WindowNext) ||
		chord.ActiveProfile != "chrome" || chord.BindingProfile != "default" || chord.Outcome != trace.Success {
		t.Fatalf("chord observation = %+v", chord)
	}
	if !reflect.DeepEqual(desktop.actions, []core.Action{core.WindowCycleNext, core.WindowCycleCommit}) {
		t.Fatalf("desktop actions = %v", desktop.actions)
	}
}

func TestDisabledTriggerChordKeepsExistingBaseBindingBehavior(t *testing.T) {
	settings := config.Default()
	settings.Bindings = map[string]map[string]string{"chrome": {"lt+rb": ""}}
	desktop := &fakeDesktop{profile: "chrome"}
	recorder := &fakeTraceRecorder{}
	controller := New(settings, fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)

	if err := controller.Step(core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, 1.0/120, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(desktop.actions, []core.Action{core.TabNext}) {
		t.Fatalf("desktop actions = %v", desktop.actions)
	}
	if len(recorder.observations) != 2 || recorder.observations[1].Gesture != "rb" || recorder.observations[1].Action != string(core.TabNext) {
		t.Fatalf("trace facts = %+v", recorder.observations)
	}
}

func TestTraceRecordsFailureOnceAndConsumesEdge(t *testing.T) {
	desktop := &fakeDesktop{profile: "default", performError: errors.New("injected failure")}
	recorder := &fakeTraceRecorder{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Now()
	state := core.State{Buttons: core.DPadUp}

	if err := controller.Step(state, 1.0/120, now); err == nil {
		t.Fatal("expected action failure")
	}
	if err := controller.Step(state, 1.0/120, now.Add(time.Millisecond)); err != nil {
		t.Fatalf("held failed edge was retried: %v", err)
	}
	if len(recorder.observations) != 1 || recorder.observations[0].Outcome != trace.Failure {
		t.Fatalf("failure observations = %+v", recorder.observations)
	}
}

func TestTraceDoesNotCountVoiceDeleteRepeats(t *testing.T) {
	desktop := &fakeDesktop{profile: "codex"}
	recorder := &fakeTraceRecorder{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Now()
	states := []struct {
		state core.State
		at    time.Time
	}{
		{core.State{Buttons: core.Y}, now},
		{core.State{}, now.Add(time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(2 * time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(330 * time.Millisecond)},
		{core.State{Buttons: core.B}, now.Add(410 * time.Millisecond)},
		{core.State{}, now.Add(500 * time.Millisecond)},
	}
	for _, item := range states {
		if err := controller.Step(item.state, 1.0/120, item.at); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.observations) != 2 {
		t.Fatalf("voice repeat was counted: %+v", recorder.observations)
	}
	if recorder.observations[1].Gesture != "voice+b" || recorder.observations[1].Action != string(core.Backspace) {
		t.Fatalf("voice delete observation = %+v", recorder.observations[1])
	}
}

func TestTraceRecordsAnalogInactiveToActiveEdgesAndDisconnectReset(t *testing.T) {
	desktop := &fakeDesktop{profile: "default"}
	recorder := &fakeTraceRecorder{}
	controller := New(config.Default(), fakeGamepad{}, desktop, false, nil)
	controller.SetTraceSink(recorder)
	now := time.Now()
	active := core.State{LeftTrigger: 1, RightTrigger: 1, LeftX: 1, RightX: 1}
	states := []core.State{active, active, {}, active}
	for index, state := range states {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	controller.disconnect()
	if err := controller.Step(active, 1.0/120, now.Add(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(recorder.observations) != 12 {
		t.Fatalf("analog observations = %+v", recorder.observations)
	}
	wantControls := []string{"lt", "rt", "left_stick", "right_stick"}
	for group := 0; group < 3; group++ {
		for index, control := range wantControls {
			observation := recorder.observations[group*len(wantControls)+index]
			if observation.Control != control || observation.Gesture != control || observation.Resolution != trace.Observed || observation.Outcome != trace.NoOutcome {
				t.Fatalf("analog observation = %+v", observation)
			}
		}
	}
}

func TestTraceRecordsIndividualStartAndBackEdges(t *testing.T) {
	now := time.Now()
	controller := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
	recorder := &fakeTraceRecorder{}
	controller.SetTraceSink(recorder)
	for index, state := range []core.State{{Buttons: core.Start}, {}, {Buttons: core.Back}} {
		if err := controller.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.observations) != 2 || recorder.observations[0].Control != "start" || recorder.observations[1].Control != "back" {
		t.Fatalf("single system-button observations = %+v", recorder.observations)
	}
	for _, observation := range recorder.observations {
		if observation.Resolution != trace.Unbound || observation.Outcome != trace.NoOutcome {
			t.Fatalf("single system button = %+v", observation)
		}
	}
}

func TestTraceRecordsBriefBackStartAsPhysicalEdgesWithoutSystemCombo(t *testing.T) {
	settings := config.Default()
	settings.ExitHoldSeconds = 0.1
	now := time.Now()
	controller := New(settings, fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
	recorder := &fakeTraceRecorder{}
	controller.SetTraceSink(recorder)
	combo := core.State{Buttons: core.Back | core.Start}

	for _, step := range []struct {
		state core.State
		at    time.Time
	}{
		{state: combo, at: now},
		{state: combo, at: now.Add(50 * time.Millisecond)},
		{state: core.State{}, at: now.Add(60 * time.Millisecond)},
	} {
		if err := controller.Step(step.state, 1.0/120, step.at); err != nil {
			t.Fatal(err)
		}
	}

	if len(recorder.observations) != 2 {
		t.Fatalf("brief Back+Start observations = %+v", recorder.observations)
	}
	for index, want := range []string{"back", "start"} {
		observation := recorder.observations[index]
		if observation.Control != want || observation.Gesture != want || observation.Resolution != trace.Unbound || observation.Outcome != trace.NoOutcome {
			t.Fatalf("brief Back+Start observation %d = %+v", index, observation)
		}
	}
}

func TestTraceRecordsSequentialBackStartEdgesAndSystemComboAtThreshold(t *testing.T) {
	now := time.Now()

	settings := config.Default()
	settings.ExitHoldSeconds = 0.1
	controller := New(settings, fakeGamepad{}, &fakeDesktop{profile: "codex", processName: "ChatGPT.exe"}, false, nil)
	recorder := &fakeTraceRecorder{}
	controller.SetTraceSink(recorder)
	if err := controller.Step(core.State{Buttons: core.Back}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	combo := core.State{Buttons: core.Back | core.Start}
	comboStarted := now.Add(10 * time.Millisecond)
	if err := controller.Step(combo, 1.0/120, comboStarted); err != nil {
		t.Fatal(err)
	}
	exitAt := comboStarted.Add(200 * time.Millisecond)
	if err := controller.Step(combo, 1.0/120, exitAt); !errors.Is(err, ErrExitRequested) {
		t.Fatalf("emergency exit error = %v", err)
	}
	if err := controller.Step(combo, 1.0/120, exitAt.Add(time.Millisecond)); err != nil {
		t.Fatalf("emergency exit was emitted repeatedly: %v", err)
	}
	if len(recorder.observations) != 3 {
		t.Fatalf("emergency exit observations = %+v", recorder.observations)
	}
	for index, want := range []string{"back", "start"} {
		observation := recorder.observations[index]
		if observation.ForegroundApp != "ChatGPT.exe" || observation.Control != want || observation.Gesture != want || observation.Resolution != trace.Unbound || observation.Outcome != trace.NoOutcome {
			t.Fatalf("physical system-button observation %d = %+v", index, observation)
		}
	}
	observation := recorder.observations[2]
	if !observation.At.Equal(exitAt) || observation.ForegroundApp != "ChatGPT.exe" || observation.ActiveProfile != "codex" || observation.Control != "back+start" ||
		observation.Gesture != "back+start" || observation.Action != "emergency_exit" ||
		observation.Resolution != trace.System || observation.Outcome != trace.Success {
		t.Fatalf("emergency exit observation = %+v", observation)
	}
}
