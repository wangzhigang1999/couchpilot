package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/usage"
)

func hasTraceFlag(value, want string) bool {
	for _, flag := range strings.Split(value, ",") {
		if flag == want {
			return true
		}
	}
	return false
}

func TestStrategyRevisionTracksBindingsAndVoiceSemantics(t *testing.T) {
	settings := config.Default()
	first := New(settings, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision()
	second := New(settings, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision()
	if first == "" || first != second {
		t.Fatalf("strategy revision is not stable: %q %q", first, second)
	}

	withBinding := settings
	withBinding.Bindings = map[string]map[string]string{"default": {"a": string(core.Escape)}}
	if got := New(withBinding, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision(); got == first {
		t.Fatal("binding change did not change strategy revision")
	}
	withVoiceMode := settings
	withVoiceMode.VoiceMode = "hold"
	if got := New(withVoiceMode, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision(); got == first {
		t.Fatal("voice mode change did not change strategy revision")
	}
	withTimeout := settings
	withTimeout.VoiceSubmitTimeoutSeconds++
	if got := New(withTimeout, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision(); got == first {
		t.Fatal("voice timeout change did not change strategy revision")
	}
	withDeadzone := settings
	withDeadzone.Deadzone += 0.01
	if got := New(withDeadzone, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision(); got == first {
		t.Fatal("deadzone change did not change strategy revision")
	}
	withPollRate := settings
	withPollRate.PollHz++
	if got := New(withPollRate, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision(); got == first {
		t.Fatal("poll rate change did not change strategy revision")
	}
	withProfiles := settings
	withProfiles.AppProfiles = append([]core.AppProfile(nil), settings.AppProfiles...)
	withProfiles.AppProfiles[0], withProfiles.AppProfiles[1] = withProfiles.AppProfiles[1], withProfiles.AppProfiles[0]
	if got := New(withProfiles, fakeGamepad{}, &fakeDesktop{}, false, nil).StrategyRevision(); got == first {
		t.Fatal("app profile routing change did not change strategy revision")
	}
	inventory := NewResolver(settings.Bindings).BindingInventory()
	if got := strategyIDForVersion(settings, inventory, "future"); got == first {
		t.Fatal("trace semantics version change did not change strategy revision")
	}
}

func TestTraceEmitsOneInputAttemptPerDigitalRisingEdgeAndOneHold(t *testing.T) {
	recorder := &fakeUsageRecorder{}
	engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
	engine.SetUsageRecorder(recorder)
	now := time.Now()
	for index, state := range []core.State{{Buttons: core.A}, {Buttons: core.A}, {}} {
		if err := engine.Step(state, 1.0/120, now.Add(time.Duration(index)*200*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}

	attempts := recorder.events(usage.EventInputAttempt)
	if len(attempts) != 1 {
		t.Fatalf("input attempts = %+v", attempts)
	}
	if got := attempts[0]; got.StrategyID == "" || got.Control != "a" || got.PhysicalGesture != "a" || got.GestureKind != usage.GestureSingle {
		t.Fatalf("input attempt = %+v", got)
	}
	holds := recorder.events(usage.EventHoldEpisode)
	if len(holds) != 1 || holds[0].Control != "a" || holds[0].Reason != "released" || holds[0].DurationBucket != "300_999ms" {
		t.Fatalf("hold episodes = %+v", holds)
	}
}

func TestTraceRecordsEveryActiveTriggerCandidateAndPreservesLTPriority(t *testing.T) {
	settings := config.Default()
	settings.Bindings = map[string]map[string]string{
		"default": {
			"lt+a": "",
			"rt+a": string(core.Enter),
		},
	}
	recorder := &fakeUsageRecorder{}
	desktop := &fakeDesktop{profile: "default"}
	engine := New(settings, fakeGamepad{}, desktop, false, nil)
	engine.SetUsageRecorder(recorder)
	now := time.Now()
	if err := engine.Step(core.State{Buttons: core.A, LeftTrigger: 1, RightTrigger: 1}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}

	attempts := recorder.events(usage.EventInputAttempt)
	if len(attempts) != 1 || attempts[0].Gesture != "a" || attempts[0].Action != string(core.ClickLeft) || attempts[0].PhysicalGesture != "lt+rt+a" {
		t.Fatalf("LT-priority input attempt = %+v", attempts)
	}
	probes := recorder.events(usage.EventChordProbe)
	if len(probes) != 2 {
		t.Fatalf("chord probes = %+v", probes)
	}
	byGesture := map[string]usage.Observation{}
	for _, probe := range probes {
		byGesture[probe.Gesture] = probe
	}
	left := byGesture["lt+a"]
	if left.PhysicalGesture != "lt+a" || left.CandidateResolution != usage.ResolutionDisabled || !hasTraceFlag(left.Flags, "fallback") || !hasTraceFlag(left.Flags, "dual_trigger") {
		t.Fatalf("LT probe = %+v", left)
	}
	right := byGesture["rt+a"]
	if right.CandidateResolution != usage.ResolutionBound || !hasTraceFlag(right.Flags, "priority_blocked") || !hasTraceFlag(right.Flags, "fallback") {
		t.Fatalf("RT probe = %+v", right)
	}
	if hasTraceFlag(left.Flags, "selected") || hasTraceFlag(right.Flags, "selected") {
		t.Fatalf("unexpected selected probe: %+v", probes)
	}
}

func TestTraceRecordsSelectedChordLeadAndModifierRole(t *testing.T) {
	recorder := &fakeUsageRecorder{}
	engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "chrome"}, false, nil)
	engine.SetUsageRecorder(recorder)
	now := time.Now()
	steps := []struct {
		state core.State
		at    time.Time
	}{
		{core.State{LeftTrigger: 1, LeftX: 1}, now},
		{core.State{Buttons: core.RightShoulder, LeftTrigger: 1, LeftX: 1}, now.Add(200 * time.Millisecond)},
		{core.State{}, now.Add(500 * time.Millisecond)},
	}
	for _, step := range steps {
		if err := engine.Step(step.state, 1.0/120, step.at); err != nil {
			t.Fatal(err)
		}
	}

	probes := recorder.events(usage.EventChordProbe)
	if len(probes) != 1 || probes[0].Gesture != "lt+rb" || probes[0].PhysicalGesture != "lt+rb" || probes[0].IntervalBucket != "150_399ms" || !hasTraceFlag(probes[0].Flags, "selected") {
		t.Fatalf("selected chord probe = %+v", probes)
	}
	attempts := recorder.events(usage.EventInputAttempt)
	if len(attempts) != 1 || attempts[0].PhysicalGesture != "lt+rb" || attempts[0].GestureKind != usage.GestureTriggerChord {
		t.Fatalf("chord input attempt = %+v", attempts)
	}
	var triggerHold *usage.Observation
	for _, observation := range recorder.events(usage.EventHoldEpisode) {
		if observation.Control == "lt" {
			copy := observation
			triggerHold = &copy
		}
	}
	if triggerHold == nil || triggerHold.Flags != "pointer_and_candidate" || triggerHold.CountBucket != "1" {
		t.Fatalf("trigger hold = %+v", triggerHold)
	}
}

func TestTraceRecordsLateModifierWithoutSecondInputAttempt(t *testing.T) {
	recorder := &fakeUsageRecorder{}
	engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "codex"}, false, nil)
	engine.SetUsageRecorder(recorder)
	now := time.Now()
	if err := engine.Step(core.State{Buttons: core.A}, 1.0/120, now); err != nil {
		t.Fatal(err)
	}
	if err := engine.Step(core.State{Buttons: core.A, RightTrigger: 1}, 1.0/120, now.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	attempts := recorder.events(usage.EventInputAttempt)
	if len(attempts) != 1 || attempts[0].Gesture != "a" {
		t.Fatalf("late modifier created another input attempt: %+v", attempts)
	}
	probes := recorder.events(usage.EventChordProbe)
	if len(probes) != 1 {
		t.Fatalf("late modifier probes = %+v", probes)
	}
	probe := probes[0]
	if probe.Gesture != "rt+a" || probe.PhysicalGesture != "rt+a" || probe.CandidateResolution != usage.ResolutionBound ||
		probe.IntervalBucket != "50_149ms" || !hasTraceFlag(probe.Flags, "late_modifier") {
		t.Fatalf("late modifier probe = %+v", probe)
	}
}

func TestChordProbeMarksPointerContextWithoutChangingResolution(t *testing.T) {
	t.Run("trigger was previously used for precision pointer movement", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		if err := engine.Step(core.State{LeftTrigger: 1, LeftX: 1}, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{Buttons: core.A, LeftTrigger: 1}, 1.0/120, now.Add(100*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		probes := recorder.events(usage.EventChordProbe)
		if len(probes) != 1 || probes[0].Gesture != "lt+a" || probes[0].CandidateResolution != usage.ResolutionUnbound || !hasTraceFlag(probes[0].Flags, "pointer_context") {
			t.Fatalf("precision-pointer probe = %+v", probes)
		}
		if hasTraceFlag(probes[0].Flags, "left_stick_active") || hasTraceFlag(probes[0].Flags, "right_stick_active") {
			t.Fatalf("historical pointer context was confused with current stick state: %+v", probes[0])
		}
	})

	t.Run("late modifier with stick active in the same frame", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "codex"}, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		if err := engine.Step(core.State{Buttons: core.A}, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{Buttons: core.A, RightTrigger: 1, RightY: 1}, 1.0/120, now.Add(100*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		probes := recorder.events(usage.EventChordProbe)
		if len(probes) != 1 || !hasTraceFlag(probes[0].Flags, "late_modifier") || !hasTraceFlag(probes[0].Flags, "right_stick_active") || !hasTraceFlag(probes[0].Flags, "pointer_context") {
			t.Fatalf("same-frame pointer-context probe = %+v", probes)
		}
	})
}

func TestTraceComposeSessionRecordsSubmitTimeoutAndRepeat(t *testing.T) {
	t.Run("submit with repeat summary", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "codex"}, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		steps := []struct {
			state core.State
			at    time.Time
		}{
			{core.State{Buttons: core.Y}, now},
			{core.State{}, now.Add(time.Millisecond)},
			{core.State{Buttons: core.B}, now.Add(2 * time.Millisecond)},
			{core.State{Buttons: core.B}, now.Add(330 * time.Millisecond)},
			{core.State{Buttons: core.B}, now.Add(410 * time.Millisecond)},
			{core.State{}, now.Add(500 * time.Millisecond)},
			{core.State{Buttons: core.A}, now.Add(2 * time.Second)},
		}
		for _, step := range steps {
			if err := engine.Step(step.state, 1.0/120, step.at); err != nil {
				t.Fatal(err)
			}
		}
		repeats := recorder.events(usage.EventRepeatEpisode)
		if len(repeats) != 1 || repeats[0].Gesture != "voice+b" || repeats[0].CountBucket != "2" || repeats[0].Reason != "released" {
			t.Fatalf("repeat episodes = %+v", repeats)
		}
		sessions := recorder.events(usage.EventComposeSession)
		if len(sessions) != 1 || sessions[0].Reason != "submit_succeeded" || sessions[0].Outcome != usage.OutcomeSuccess || !hasTraceFlag(sessions[0].Flags, "deletes_1") || !hasTraceFlag(sessions[0].Flags, "repeats_2") {
			t.Fatalf("compose sessions = %+v", sessions)
		}
		attempts := recorder.events(usage.EventInputAttempt)
		if got := attempts[len(attempts)-1]; got.Gesture != "voice+a" || got.PhysicalGesture != "voice+a" || got.GestureKind != usage.GestureModeSequence {
			t.Fatalf("compose submit attempt = %+v", got)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		settings := config.Default()
		settings.VoiceSubmitTimeoutSeconds = 5
		recorder := &fakeUsageRecorder{}
		engine := New(settings, fakeGamepad{}, &fakeDesktop{profile: "codex"}, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		if err := engine.Step(core.State{Buttons: core.Y}, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{}, 1.0/120, now.Add(6*time.Second)); err != nil {
			t.Fatal(err)
		}
		sessions := recorder.events(usage.EventComposeSession)
		if len(sessions) != 1 || sessions[0].Reason != "timeout" || sessions[0].DurationBucket != "3_9s" {
			t.Fatalf("timeout sessions = %+v", sessions)
		}
	})
}

func TestTraceFailureSessionsKeepTheRootDispatchOutcome(t *testing.T) {
	t.Run("voice submit dispatch failure", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		desktop := &fakeDesktop{profile: "codex"}
		desktop.performHook = func(action core.Action, _ int) error {
			if action == core.Enter {
				return errors.New("submit failed")
			}
			return nil
		}
		engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		if err := engine.Step(core.State{Buttons: core.Y}, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{}, 1.0/120, now.Add(time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{Buttons: core.A}, 1.0/120, now.Add(time.Second)); err == nil {
			t.Fatal("expected submit dispatch failure")
		}
		sessions := recorder.events(usage.EventComposeSession)
		if len(sessions) != 1 || sessions[0].Reason != "submit_dispatch_failed" || sessions[0].Outcome != usage.OutcomeFailure {
			t.Fatalf("submit failure session = %+v", sessions)
		}
		attempts := recorder.events(usage.EventInputAttempt)
		if got := attempts[len(attempts)-1]; got.Gesture != "voice+a" || got.Outcome != usage.OutcomeFailure {
			t.Fatalf("submit failure attempt = %+v", got)
		}
		if engine.composeProfile != "" {
			t.Fatalf("compose remained armed after submit failure: %q", engine.composeProfile)
		}
	})

	t.Run("initial delete dispatch failure", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		desktop := &fakeDesktop{profile: "codex"}
		desktop.performHook = func(action core.Action, _ int) error {
			if action == core.Backspace {
				return errors.New("delete failed")
			}
			return nil
		}
		engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		if err := engine.Step(core.State{Buttons: core.Y}, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{}, 1.0/120, now.Add(time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if err := engine.Step(core.State{Buttons: core.B}, 1.0/120, now.Add(2*time.Millisecond)); err == nil {
			t.Fatal("expected initial delete dispatch failure")
		}
		sessions := recorder.events(usage.EventComposeSession)
		if len(sessions) != 1 || sessions[0].Reason != "delete_dispatch_failed" || sessions[0].Outcome != usage.OutcomeFailure {
			t.Fatalf("delete failure session = %+v", sessions)
		}
		if len(recorder.events(usage.EventRepeatEpisode)) != 0 {
			t.Fatalf("repeat started after initial delete failure: %+v", recorder.events(usage.EventRepeatEpisode))
		}
		if engine.composeProfile != "" {
			t.Fatalf("compose remained armed after delete failure: %q", engine.composeProfile)
		}
	})

	t.Run("repeat dispatch failure", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		desktop := &fakeDesktop{profile: "codex"}
		backspaces := 0
		desktop.performHook = func(action core.Action, _ int) error {
			if action == core.Backspace {
				backspaces++
				if backspaces == 2 {
					return errors.New("repeat failed")
				}
			}
			return nil
		}
		engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		steps := []struct {
			state core.State
			at    time.Time
		}{
			{core.State{Buttons: core.Y}, now},
			{core.State{}, now.Add(time.Millisecond)},
			{core.State{Buttons: core.B}, now.Add(2 * time.Millisecond)},
		}
		for _, step := range steps {
			if err := engine.Step(step.state, 1.0/120, step.at); err != nil {
				t.Fatal(err)
			}
		}
		if err := engine.Step(core.State{Buttons: core.B}, 1.0/120, now.Add(330*time.Millisecond)); err == nil {
			t.Fatal("expected repeat dispatch failure")
		}
		sessions := recorder.events(usage.EventComposeSession)
		if len(sessions) != 1 || sessions[0].Reason != "repeat_dispatch_failure" || sessions[0].Outcome != usage.OutcomeFailure {
			t.Fatalf("repeat failure compose session = %+v", sessions)
		}
		repeats := recorder.events(usage.EventRepeatEpisode)
		if len(repeats) != 1 || repeats[0].Reason != "repeat_dispatch_failure" || repeats[0].Outcome != usage.OutcomeFailure || repeats[0].CountBucket != "0" {
			t.Fatalf("repeat failure episode = %+v", repeats)
		}
		if engine.composeProfile != "" || engine.repeatButton != 0 {
			t.Fatalf("repeat failure left active state: compose=%q repeat=%v", engine.composeProfile, engine.repeatButton)
		}
	})

	t.Run("window cycle failure survives successful cleanup commit", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		desktop := &fakeDesktop{profile: "default"}
		desktop.performHook = func(action core.Action, _ int) error {
			if action == core.WindowCycleNext {
				return errors.New("cycle failed")
			}
			return nil
		}
		engine := New(config.Default(), fakeGamepad{}, desktop, false, nil)
		engine.SetUsageRecorder(recorder)
		if err := engine.Step(core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, 1.0/120, time.Now()); err == nil {
			t.Fatal("expected window cycle failure")
		}
		sessions := recorder.events(usage.EventWindowSession)
		if len(sessions) != 1 || sessions[0].Reason != "cycle_failure" || sessions[0].Outcome != usage.OutcomeFailure {
			t.Fatalf("window failure session = %+v", sessions)
		}
		if len(desktop.actions) != 2 || desktop.actions[0] != core.WindowCycleNext || desktop.actions[1] != core.WindowCycleCommit {
			t.Fatalf("window cleanup actions = %v", desktop.actions)
		}
		if engine.windowSwitching || engine.trace.window.active {
			t.Fatal("window failure left active switching state")
		}
	})
}

func TestTraceWindowSessionSummarizesMixedDirectionAndDisconnect(t *testing.T) {
	t.Run("mixed direction", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		states := []core.State{
			{Buttons: core.RightShoulder, LeftTrigger: 1},
			{LeftTrigger: 1},
			{Buttons: core.LeftShoulder, LeftTrigger: 1},
			{},
		}
		for index, state := range states {
			if err := engine.Step(state, 1.0/120, now.Add(time.Duration(index)*100*time.Millisecond)); err != nil {
				t.Fatal(err)
			}
		}
		sessions := recorder.events(usage.EventWindowSession)
		if len(sessions) != 1 || sessions[0].CountBucket != "2" || sessions[0].Reason != "trigger_released" || !hasTraceFlag(sessions[0].Flags, "mixed") {
			t.Fatalf("window sessions = %+v", sessions)
		}
	})

	t.Run("disconnect closes active state", func(t *testing.T) {
		recorder := &fakeUsageRecorder{}
		engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
		engine.SetUsageRecorder(recorder)
		now := time.Now()
		if err := engine.Step(core.State{Buttons: core.RightShoulder, LeftTrigger: 1}, 1.0/120, now); err != nil {
			t.Fatal(err)
		}
		engine.disconnect()
		windows := recorder.events(usage.EventWindowSession)
		if len(windows) != 1 || windows[0].Reason != "disconnect" {
			t.Fatalf("disconnect window sessions = %+v", windows)
		}
		var sawButton, sawTrigger bool
		for _, hold := range recorder.events(usage.EventHoldEpisode) {
			if hold.Control == "rb" && hold.Reason == "disconnect" {
				sawButton = true
			}
			if hold.Control == "lt" && hold.Reason == "disconnect" {
				sawTrigger = true
			}
		}
		if !sawButton || !sawTrigger {
			t.Fatalf("disconnect holds = %+v", recorder.events(usage.EventHoldEpisode))
		}
	})
}

func TestTraceClassifiesPhysicalStickActivationsAndSimultaneousButtons(t *testing.T) {
	recorder := &fakeUsageRecorder{}
	engine := New(config.Default(), fakeGamepad{}, &fakeDesktop{profile: "default"}, false, nil)
	engine.SetUsageRecorder(recorder)
	now := time.Now()
	states := []core.State{
		{RightX: 1},
		{},
		{RightY: 1},
		{},
		{Buttons: core.A | core.B},
	}
	for index, state := range states {
		if err := engine.Step(state, 1.0/120, now.Add(time.Duration(index)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	var stickFlags []string
	for _, event := range recorder.events(usage.EventPhysicalActivation) {
		if event.Control == "right_stick" {
			stickFlags = append(stickFlags, event.Flags)
		}
	}
	if len(stickFlags) != 2 || stickFlags[0] != "horizontal_only" || stickFlags[1] != "vertical_only" {
		t.Fatalf("right-stick classifications = %v", stickFlags)
	}
	attempts := recorder.events(usage.EventInputAttempt)
	if len(attempts) != 2 {
		t.Fatalf("simultaneous attempts = %+v", attempts)
	}
	for _, attempt := range attempts {
		if attempt.PhysicalGesture != "a+b" || !hasTraceFlag(attempt.Flags, "simultaneous_buttons") {
			t.Fatalf("simultaneous attempt = %+v", attempt)
		}
	}
}
