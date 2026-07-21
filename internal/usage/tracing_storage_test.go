package usage

import (
	"fmt"
	htmlstd "html"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTraceProcessorDerivesTransitionAndOneContextSession(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)

	first := Observation{
		Sequence: 1, At: now, Kind: EventInputAttempt, StrategyID: "s-one",
		ForegroundApp: "ChatGPT.exe", ActiveProfile: "codex", Control: "rb", Gesture: "rb",
		Action: "codex_next_task", Resolution: ResolutionBound, Outcome: OutcomeSuccess,
	}
	second := first
	second.Sequence = 2
	second.At = now.Add(300 * time.Millisecond)
	second.Control = "lb"
	second.Gesture = "lb"
	second.Action = "codex_previous_task"
	if _, err := processor.add(pending, first, now); err != nil {
		t.Fatal(err)
	}
	if _, err := processor.add(pending, second, now); err != nil {
		t.Fatal(err)
	}

	var inputs, transitions, sessions uint64
	for key, value := range pending {
		switch key.Entry.Kind {
		case EventInputAttempt:
			inputs += value.Attempts
		case EventTransition:
			transitions += value.Attempts
			if key.Entry.RelatedAction != "codex_next_task" || key.Entry.Action != "codex_previous_task" || key.Entry.IntervalBucket != "250_750ms" {
				t.Fatalf("transition key = %#v", key.Entry)
			}
		case EventContextSession:
			sessions += value.Attempts
		}
	}
	if inputs != 2 || transitions != 1 || sessions != 1 {
		t.Fatalf("input/transition/session = %d/%d/%d", inputs, transitions, sessions)
	}
}

func TestTraceProcessorBreaksSequenceOnGapAndSameFrame(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	input := func(sequence uint64, at time.Time, action string) Observation {
		return Observation{
			Sequence: sequence, At: at, Kind: EventInputAttempt, StrategyID: "s-one",
			ForegroundApp: "app.exe", ActiveProfile: "default", Control: "a", Gesture: "a",
			Action: action, Resolution: ResolutionBound, Outcome: OutcomeSuccess,
		}
	}
	for _, observation := range []Observation{
		input(1, now, "arrow_up"),
		input(2, now, "arrow_down"),                  // Same poll frame has no reliable order.
		input(4, now.Add(time.Second), "arrow_left"), // Sequence 3 was dropped.
		input(5, now.Add(1500*time.Millisecond), "arrow_right"),
	} {
		if _, err := processor.add(pending, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	var transitions uint64
	for key, value := range pending {
		if key.Entry.Kind == EventTransition {
			transitions += value.Attempts
		}
	}
	if transitions != 1 {
		t.Fatalf("transitions = %d, want only arrow_left -> arrow_right", transitions)
	}
	if processor.health.SequenceBreaks != 1 {
		t.Fatalf("sequence breaks = %d, want 1", processor.health.SequenceBreaks)
	}
}

func TestSimultaneousButtonsDoNotCreatePriorOrFutureTransitions(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	input := func(sequence uint64, at time.Time, control, action, flags string) Observation {
		return Observation{
			Sequence: sequence, At: at, Kind: EventInputAttempt, StrategyID: "s-one",
			ForegroundApp: "app.exe", ActiveProfile: "default", Control: control, Gesture: control,
			PhysicalGesture: "a+b", Action: action, Resolution: ResolutionBound,
			Outcome: OutcomeSuccess, Flags: flags,
		}
	}
	for _, observation := range []Observation{
		input(1, now, "x", "copy", ""),
		input(2, now.Add(100*time.Millisecond), "a", "click_left", "simultaneous_buttons"),
		input(3, now.Add(100*time.Millisecond), "b", "escape", "simultaneous_buttons"),
		input(4, now.Add(200*time.Millisecond), "y", "paste", ""),
	} {
		if _, err := processor.add(pending, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	for key := range pending {
		if key.Entry.Kind == EventTransition {
			t.Fatalf("simultaneous input invented a transition: %#v", key.Entry)
		}
	}
}

func TestOnlySuccessfulDispatchesParticipateInTransitions(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	input := func(sequence uint64, at time.Time, action string, outcome Outcome) Observation {
		return Observation{
			Sequence: sequence, At: at, Kind: EventInputAttempt, StrategyID: "s-one",
			ForegroundApp: "app.exe", ActiveProfile: "default", Control: "a", Gesture: "a",
			Action: action, Resolution: ResolutionBound, Outcome: outcome,
		}
	}
	for _, observation := range []Observation{
		input(1, now, "arrow_up", OutcomeSuccess),
		input(2, now.Add(100*time.Millisecond), "arrow_down", OutcomeFailure),
		input(3, now.Add(200*time.Millisecond), "arrow_left", OutcomeSuccess),
		input(4, now.Add(300*time.Millisecond), "arrow_right", OutcomeSuccess),
		input(5, now.Add(400*time.Millisecond), "copy", OutcomeNone),
		input(6, now.Add(500*time.Millisecond), "paste", OutcomeSuccess),
		input(7, now.Add(600*time.Millisecond), "enter", OutcomeSuccess),
	} {
		if _, err := processor.add(pending, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	transitions := make(map[string]uint64)
	var sessions uint64
	for key, value := range pending {
		if key.Entry.Kind == EventTransition {
			transitions[key.Entry.RelatedAction+"->"+key.Entry.Action] += value.Attempts
		}
		if key.Entry.Kind == EventContextSession {
			sessions += value.Attempts
		}
	}
	if len(transitions) != 2 || transitions["arrow_left->arrow_right"] != 1 || transitions["paste->enter"] != 1 {
		t.Fatalf("success-only transitions = %#v", transitions)
	}
	if sessions != 1 {
		t.Fatalf("context sessions = %d, want failure/none to retain the existing session", sessions)
	}
}

func TestPhysicalActivationStartsContextExposureWithoutInputTransition(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	physical := Observation{
		Sequence: 1, At: now, Kind: EventPhysicalActivation, StrategyID: "s-one",
		ForegroundApp: "notes.exe", ActiveProfile: "notes", Control: "left_stick",
		Gesture: "left_stick", Resolution: ResolutionObserved, Outcome: OutcomeNone,
	}
	input := Observation{
		Sequence: 2, At: now.Add(100 * time.Millisecond), Kind: EventInputAttempt, StrategyID: "s-one",
		ForegroundApp: "notes.exe", ActiveProfile: "notes", Control: "rb", Gesture: "rb",
		Action: "tab_next", Resolution: ResolutionBound, Outcome: OutcomeSuccess,
	}
	if _, err := processor.add(pending, physical, now); err != nil {
		t.Fatal(err)
	}
	if _, err := processor.add(pending, input, now); err != nil {
		t.Fatal(err)
	}
	var sessions, transitions uint64
	for key, value := range pending {
		if key.Entry.Kind == EventContextSession {
			sessions += value.Attempts
		}
		if key.Entry.Kind == EventTransition {
			transitions += value.Attempts
		}
	}
	if sessions != 1 || transitions != 0 {
		t.Fatalf("sessions/transitions = %d/%d", sessions, transitions)
	}
}

func TestTriggerActivationPreservesActionToChordTransitionButStickBreaksIt(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	observations := []Observation{
		{Sequence: 1, At: now, Kind: EventInputAttempt, StrategyID: "s-one", ForegroundApp: "app.exe", ActiveProfile: "default", Control: "a", Gesture: "a", Action: "click_left", Resolution: ResolutionBound, Outcome: OutcomeSuccess},
		{Sequence: 2, At: now.Add(50 * time.Millisecond), Kind: EventPhysicalActivation, StrategyID: "s-one", ForegroundApp: "app.exe", ActiveProfile: "default", Control: "lt", Gesture: "lt", Resolution: ResolutionObserved, Outcome: OutcomeNone},
		{Sequence: 3, At: now.Add(100 * time.Millisecond), Kind: EventInputAttempt, StrategyID: "s-one", ForegroundApp: "app.exe", ActiveProfile: "default", Control: "rb", Gesture: "lt+rb", Action: "window_next", Resolution: ResolutionBound, Outcome: OutcomeSuccess},
		{Sequence: 4, At: now.Add(150 * time.Millisecond), Kind: EventPhysicalActivation, StrategyID: "s-one", ForegroundApp: "app.exe", ActiveProfile: "default", Control: "left_stick", Gesture: "left_stick", Resolution: ResolutionObserved, Outcome: OutcomeNone},
		{Sequence: 5, At: now.Add(200 * time.Millisecond), Kind: EventInputAttempt, StrategyID: "s-one", ForegroundApp: "app.exe", ActiveProfile: "default", Control: "b", Gesture: "b", Action: "escape", Resolution: ResolutionBound, Outcome: OutcomeSuccess},
	}
	for _, observation := range observations {
		if _, err := processor.add(pending, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	var transitions uint64
	for key, value := range pending {
		if key.Entry.Kind != EventTransition {
			continue
		}
		transitions += value.Attempts
		if key.Entry.RelatedAction != "click_left" || key.Entry.Action != "window_next" {
			t.Fatalf("unexpected transition across trigger/stick: %#v", key.Entry)
		}
	}
	if transitions != 1 {
		t.Fatalf("transitions = %d, want action -> trigger chord only", transitions)
	}
}

func TestTracingFieldsSurviveSnapshotAndWALRoundTrip(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	inventory := []BindingDefinition{{Profile: "default", Gesture: "lt+a", Action: "enter", Resolution: ResolutionBound}}
	recorder, err := Open(Options{
		Directory: directory, Inventory: inventory, StrategyID: "s-test",
		Now: func() time.Time { return now }, FlushInterval: time.Hour, MaxBatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Observation{
		At: now, Kind: EventChordProbe, StrategyID: "s-test", ForegroundApp: `C:\Programs\ChatGPT.exe`,
		ActiveProfile: "codex", BindingProfile: "default", Control: "lt+a",
		PhysicalGesture: "lt+a", Gesture: "lt+a", GestureKind: GestureTriggerChord,
		Action: "enter", RelatedGesture: "a", RelatedAction: "click_left",
		Resolution: ResolutionObserved, CandidateResolution: ResolutionDisabled,
		Outcome: OutcomeNone, IntervalBucket: "150_399ms", DurationBucket: "100_299ms",
		CountBucket: "1", Reason: "fallback", Flags: "fallback",
	})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	persisted := readPersistedSnapshot(t, directory)
	if len(persisted.Strategies) != 1 || persisted.Strategies[0].ID != "s-test" {
		t.Fatalf("strategies = %#v", persisted.Strategies)
	}
	var got *snapshotEntry
	for index := range persisted.Entries {
		if persisted.Entries[index].Kind == EventChordProbe {
			got = &persisted.Entries[index]
			break
		}
	}
	if got == nil || got.ForegroundApp != "ChatGPT.exe" || got.PhysicalGesture != "lt+a" ||
		got.CandidateResolution != ResolutionDisabled || got.IntervalBucket != "150_399ms" ||
		got.DurationBucket != "100_299ms" || got.Reason != "fallback" {
		t.Fatalf("tracing snapshot entry = %#v", got)
	}
	raw, err := os.ReadFile(SnapshotPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), now.Format(time.RFC3339Nano)) || strings.Contains(string(raw), `"at"`) || strings.Contains(string(raw), `"sequence"`) {
		t.Fatalf("snapshot leaked an exact trace timeline: %s", raw)
	}
}

func TestContextExposureRestartsAfterIdleAndContextChange(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	for index, observation := range []Observation{
		{At: now, ForegroundApp: "one.exe", ActiveProfile: "default"},
		{At: now.Add(61 * time.Second), ForegroundApp: "one.exe", ActiveProfile: "default"},
		{At: now.Add(62 * time.Second), ForegroundApp: "two.exe", ActiveProfile: "default"},
	} {
		observation.Sequence = uint64(index + 1)
		observation.Kind = EventPhysicalActivation
		observation.StrategyID = "s-one"
		observation.Control = "left_stick"
		observation.Gesture = "left_stick"
		observation.Resolution = ResolutionObserved
		observation.Outcome = OutcomeNone
		if _, err := processor.add(pending, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	var sessions uint64
	reasons := make(map[string]uint64)
	for key, value := range pending {
		if key.Entry.Kind == EventContextSession {
			sessions += value.Attempts
			reasons[key.Entry.Reason] += value.Attempts
		}
	}
	if sessions != 3 || reasons["start"] != 1 || reasons["idle"] != 1 || reasons["context_changed"] != 1 {
		t.Fatalf("context sessions/reasons = %d/%v", sessions, reasons)
	}
}

func TestLegacySnapshotUsesOnlyRetainedDailyCounts(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, testLocation)
	data := []byte(`{"schema_version":1,"updated_day":"2026-07-21","applied_through":4,"inventory":[],"controls":["a"],"entries":[{"foreground_app":"legacy.exe","control":"a","gesture":"a","resolution":"observed","attempts":999,"successes":0,"failures":0,"first_used_day":"2020-01-01","last_used_day":"2026-07-21","daily":{"2020-01-01":{"attempts":998,"successes":0,"failures":0},"2026-07-21":{"attempts":1,"successes":0,"failures":0}}}],"dropped":0}`)
	state, err := decodeSnapshot(data, "legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	pruneDaily(&state, now)
	if len(state.Entries) != 1 {
		t.Fatalf("legacy entries = %d", len(state.Entries))
	}
	for _, entry := range state.Entries {
		if entry.Key.Kind != EventLegacy || entry.Attempts != 1 || entry.FirstUsedDay != "2026-07-21" {
			t.Fatalf("migrated legacy entry = %#v", entry)
		}
	}
}

func TestAppBucketsAreBasenamesAndBounded(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	now := time.Date(2026, 7, 21, 20, 0, 0, 0, testLocation)
	for index := 0; index < maximumAppBuckets+1; index++ {
		app := fmt.Sprintf(`C:\Apps\app-%02d.exe`, index)
		observation := Observation{
			Sequence: uint64(index + 1), At: now, Kind: EventHoldEpisode,
			ForegroundApp: app, Control: "a", Gesture: "a",
			Resolution: ResolutionObserved, Outcome: OutcomeNone,
		}
		if _, err := processor.add(pending, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	seenOther, seenPath := false, false
	for key := range pending {
		if key.Entry.ForegroundApp == otherAppBucket {
			seenOther = true
		}
		if strings.ContainsAny(key.Entry.ForegroundApp, `/\\`) {
			seenPath = true
		}
	}
	if !seenOther || seenPath || processor.health.Coalesced != 1 {
		t.Fatalf("other/path/coalesced = %v/%v/%d", seenOther, seenPath, processor.health.Coalesced)
	}
}

func TestOverlongAppNameIsInvalidNotPersisted(t *testing.T) {
	state := emptyAggregate()
	processor := newTraceProcessor(&state)
	pending := make(map[deltaKey]counters)
	observation := Observation{
		Sequence: 1, At: time.Now(), Kind: EventHoldEpisode,
		ForegroundApp: strings.Repeat("a", maximumAppNameBytes+1),
		Resolution:    ResolutionObserved, Outcome: OutcomeNone,
	}
	if _, err := processor.add(pending, observation, observation.At); err == nil {
		t.Fatal("expected overlong foreground app to be rejected")
	}
	if len(pending) != 0 || processor.health.InvalidDropped != 1 {
		t.Fatalf("pending/invalid = %d/%d", len(pending), processor.health.InvalidDropped)
	}
}

func TestEnsureStrategyMovesRollbackToCurrentPosition(t *testing.T) {
	state := emptyAggregate()
	ensureStrategy(&state, "s-a", []BindingDefinition{{Profile: "default", Gesture: "a", Resolution: ResolutionBound}})
	ensureStrategy(&state, "s-b", []BindingDefinition{{Profile: "default", Gesture: "b", Resolution: ResolutionBound}})
	ensureStrategy(&state, "s-a", []BindingDefinition{{Profile: "default", Gesture: "a", Action: "click_left", Resolution: ResolutionBound}})
	if len(state.Strategies) != 2 || state.Strategies[1].ID != "s-a" || state.Strategies[1].Inventory[0].Action != "click_left" {
		t.Fatalf("rollback strategy order = %#v", state.Strategies)
	}
}

func TestUnusedControlsAreScopedToCurrentStrategy(t *testing.T) {
	day := "2026-07-21"
	state := emptyAggregate()
	state.Controls = []string{"a", "b"}
	state.Strategies = []StrategyDefinition{{ID: "s-old"}, {ID: "s-current"}}
	add := func(strategy, control string) {
		key := entryKey{Kind: EventInputAttempt, StrategyID: strategy, Control: control, Gesture: control, Resolution: ResolutionObserved}
		state.Entries[key] = &aggregateEntry{Key: key, Attempts: 1, FirstUsedDay: day, LastUsedDay: day, Daily: map[string]counters{day: {Attempts: 1}}}
	}
	add("s-old", "a")
	add("s-current", "b")
	summary := summarize(state, 0)
	if strings.Join(summary.UnusedControls, ",") != "a" {
		t.Fatalf("current strategy unused controls = %#v", summary.UnusedControls)
	}
}

func TestUnusedBindingsRequireGestureSpecificExposure(t *testing.T) {
	day := "2026-07-21"
	state := emptyAggregate()
	state.Strategies = []StrategyDefinition{{ID: "s-current"}}
	state.Inventory = []BindingDefinition{
		{Profile: "default", Gesture: "lt+rb", Action: "window_next", Resolution: ResolutionBound},
		{Profile: "assistant", Gesture: "voice+a", Action: "enter", Resolution: ResolutionBound},
	}
	key := entryKey{
		Kind: EventInputAttempt, StrategyID: "s-current", ForegroundApp: "app.exe",
		ActiveProfile: "codex", Control: "a", Gesture: "a", Action: "click_left", Resolution: ResolutionBound,
	}
	state.Entries[key] = &aggregateEntry{
		Key: key, Attempts: 10, Successes: 10, FirstUsedDay: day, LastUsedDay: day,
		Daily: map[string]counters{day: {Attempts: 10, Successes: 10}},
	}
	summary := summarize(state, 0)
	if len(summary.UnusedBindings) != 0 {
		t.Fatalf("generic profile activity must not imply binding opportunity: %#v", summary.UnusedBindings)
	}
	probeKey := entryKey{
		Kind: EventChordProbe, StrategyID: "s-current", ForegroundApp: "app.exe",
		ActiveProfile: "codex", BindingProfile: "default", Gesture: "lt+rb",
		PhysicalGesture: "lt+rb", Resolution: ResolutionObserved,
		CandidateResolution: ResolutionBound, Flags: "priority_blocked",
	}
	state.Entries[probeKey] = &aggregateEntry{
		Key: probeKey, Attempts: minimumBindingOpportunities, FirstUsedDay: day, LastUsedDay: day,
		Daily: map[string]counters{day: {Attempts: minimumBindingOpportunities}},
	}
	summary = summarize(state, 0)
	if len(summary.UnusedBindings) != 1 || summary.UnusedBindings[0].Gesture != "lt+rb" {
		t.Fatalf("trigger-specific exposure = %#v", summary.UnusedBindings)
	}
	composeKey := entryKey{
		Kind: EventComposeSession, StrategyID: "s-current", ForegroundApp: "app.exe",
		ActiveProfile: "assistant", Gesture: "voice", Resolution: ResolutionObserved,
	}
	state.Entries[composeKey] = &aggregateEntry{
		Key: composeKey, Attempts: minimumBindingOpportunities, FirstUsedDay: day, LastUsedDay: day,
		Daily: map[string]counters{day: {Attempts: minimumBindingOpportunities}},
	}
	summary = summarize(state, 0)
	if len(summary.UnusedBindings) != 2 {
		t.Fatalf("compose-specific exposure = %#v", summary.UnusedBindings)
	}
}

func TestOpenMigratesLegacySchemaOneSnapshotToSchemaTwo(t *testing.T) {
	directory := t.TempDir()
	legacy := []byte(`{"schema_version":1,"updated_day":"2026-07-21","applied_through":0,"inventory":[],"controls":["a"],"entries":[{"control":"a","gesture":"a","resolution":"unbound","attempts":1,"successes":0,"failures":0,"first_used_day":"2026-07-21","last_used_day":"2026-07-21","daily":{"2026-07-21":{"attempts":1,"successes":0,"failures":0}}}],"dropped":0}`)
	if err := os.WriteFile(SnapshotPath(directory), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	recorder, err := Open(Options{Directory: directory, Now: func() time.Time {
		return time.Date(2026, 7, 21, 22, 0, 0, 0, testLocation)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	persisted := readPersistedSnapshot(t, directory)
	if persisted.SchemaVersion != schemaVersion || persisted.SchemaVersion == legacySchemaVersion || len(persisted.Entries) != 1 {
		t.Fatalf("migrated snapshot = schema %d entries %d", persisted.SchemaVersion, len(persisted.Entries))
	}
}

func TestAggregateEntryLimitCoalescesRatherThanGrowing(t *testing.T) {
	day := "2026-07-21"
	state := emptyAggregate()
	for index := 0; index < maximumEntries+100; index++ {
		key := entryKey{
			Kind: EventHoldEpisode, ForegroundApp: "app.exe", Control: "a",
			Resolution: ResolutionObserved, Reason: fmt.Sprintf("reason-%05d", index),
		}
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: 1, FirstUsedDay: day, LastUsedDay: day,
			Daily: map[string]counters{day: {Attempts: 1}},
		}
	}
	enforceAggregateLimits(&state)
	if len(state.Entries) > maximumEntries || state.Coalesced == 0 {
		t.Fatalf("entries/coalesced = %d/%d", len(state.Entries), state.Coalesced)
	}
}

func TestAggregateDailyCellLimitKeepsFrequentEntriesAndCoalescesTail(t *testing.T) {
	state := emptyAggregate()
	base := time.Date(2026, 7, 21, 12, 0, 0, 0, testLocation)
	var before uint64
	for index := 0; index < 400; index++ {
		kind := EventInputAttempt
		if index%2 != 0 {
			kind = EventHoldEpisode
		}
		key := entryKey{
			Kind: kind, ForegroundApp: "app.exe", Control: "a", Gesture: "a",
			Action: "click_left", Resolution: ResolutionObserved, Reason: fmt.Sprintf("reason-%03d", index),
		}
		daily := make(map[string]counters, recentDayCount)
		perDay := uint64(index + 1)
		for offset := 0; offset < recentDayCount; offset++ {
			day := dayString(base.AddDate(0, 0, -offset))
			daily[day] = counters{Attempts: perDay}
		}
		total := perDay * recentDayCount
		before = saturatingAdd(before, total)
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: total,
			FirstUsedDay: dayString(base.AddDate(0, 0, -(recentDayCount - 1))),
			LastUsedDay:  dayString(base), Daily: daily,
		}
	}
	enforceAggregateLimits(&state)
	if cells := dailyCellCount(state.Entries); cells > maximumDailyCells {
		t.Fatalf("daily cells = %d, maximum %d", cells, maximumDailyCells)
	}
	var after uint64
	for _, entry := range state.Entries {
		after = saturatingAdd(after, entry.Attempts)
	}
	if after != before {
		t.Fatalf("attempts changed while coalescing daily cells: %d -> %d", before, after)
	}
	for _, reason := range []string{"reason-398", "reason-399"} {
		found := false
		for key := range state.Entries {
			if key.Reason == reason {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("high-frequency entry %s was not retained", reason)
		}
	}
	primaryOverflow := overflowEntryKey(entryKey{Kind: EventInputAttempt})
	diagnosticOverflow := overflowEntryKey(entryKey{Kind: EventHoldEpisode})
	if state.Entries[primaryOverflow] == nil || state.Entries[diagnosticOverflow] == nil || state.Coalesced == 0 {
		t.Fatalf("overflow/coalesced = primary:%v diagnostic:%v count:%d", state.Entries[primaryOverflow] != nil, state.Entries[diagnosticOverflow] != nil, state.Coalesced)
	}
}

func TestSnapshotByteLimitKeepsFrequentEntriesAndPreservesCounts(t *testing.T) {
	state := emptyAggregate()
	day := "2026-07-21"
	longDimension := strings.Repeat("x", maximumDimensionBytes)
	var before uint64
	for index := 0; index < 120; index++ {
		key := entryKey{
			Kind: EventInputAttempt, StrategyID: "s-current", ForegroundApp: "app.exe",
			ActiveProfile: longDimension, BindingProfile: longDimension,
			Control: longDimension, Gesture: longDimension, PhysicalGesture: longDimension,
			Action: longDimension, Resolution: ResolutionObserved,
			Reason: fmt.Sprintf("%s-%03d", strings.Repeat("r", maximumDimensionBytes-4), index),
		}
		attempts := uint64(index + 1)
		before = saturatingAdd(before, attempts)
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: attempts, FirstUsedDay: day, LastUsedDay: day,
			Daily: map[string]counters{day: {Attempts: attempts}},
		}
	}
	const testMaximum = 48 * 1024
	data, err := enforceSnapshotByteLimit(&state, testMaximum)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > testMaximum {
		t.Fatalf("snapshot bytes = %d, maximum %d", len(data), testMaximum)
	}
	var after uint64
	highFrequencyRetained := false
	for key, entry := range state.Entries {
		after = saturatingAdd(after, entry.Attempts)
		if strings.HasSuffix(key.Reason, "-119") {
			highFrequencyRetained = true
		}
	}
	if after != before || !highFrequencyRetained || state.Coalesced == 0 {
		t.Fatalf("snapshot coalescing attempts/high/coalesced = %d->%d/%v/%d", before, after, highFrequencyRetained, state.Coalesced)
	}
}

func TestSnapshotDecoderRejectsDataAboveHardLimit(t *testing.T) {
	data := make([]byte, maximumSnapshotBytes+1)
	_, err := decodeSnapshot(data, "oversized.json")
	if err == nil || !strings.Contains(err.Error(), "snapshot exceeds") {
		t.Fatalf("decode oversized snapshot error = %v", err)
	}
}

func TestInventoryDimensionsAreBoundedBeforePersistence(t *testing.T) {
	for _, field := range []string{"profile", "gesture", "action"} {
		t.Run(field, func(t *testing.T) {
			definition := BindingDefinition{Profile: "default", Gesture: "a", Action: "click_left", Resolution: ResolutionBound}
			overlong := strings.Repeat("x", maximumDimensionBytes+1)
			switch field {
			case "profile":
				definition.Profile = overlong
			case "gesture":
				definition.Gesture = overlong
			case "action":
				definition.Action = overlong
			}
			_, err := Open(Options{Directory: t.TempDir(), Inventory: []BindingDefinition{definition}})
			if err == nil || !strings.Contains(err.Error(), "inventory "+field) || !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("Open error = %v", err)
			}
		})
	}
}

func TestChordCandidatesKeepDenominatorAndGateRecommendations(t *testing.T) {
	dayOne, dayTwo := "2026-07-20", "2026-07-21"
	state := emptyAggregate()
	state.Strategies = []StrategyDefinition{{ID: "s-current"}}
	add := func(gesture, flags string, resolution Resolution, daily map[string]counters) {
		key := entryKey{
			Kind: EventChordProbe, StrategyID: "s-current", ForegroundApp: "editor.exe",
			ActiveProfile: "editor", Gesture: gesture, PhysicalGesture: gesture,
			Resolution: ResolutionObserved, CandidateResolution: resolution, Flags: flags,
		}
		entry := &aggregateEntry{Key: key, Daily: daily}
		for day, value := range daily {
			entry.Attempts = saturatingAdd(entry.Attempts, value.Attempts)
			if entry.FirstUsedDay == "" || day < entry.FirstUsedDay {
				entry.FirstUsedDay = day
			}
			if day > entry.LastUsedDay {
				entry.LastUsedDay = day
			}
		}
		state.Entries[key] = entry
	}
	add("lt+a", "selected", ResolutionBound, map[string]counters{dayOne: {Attempts: 5}})
	add("lt+a", "fallback,priority_blocked", ResolutionUnbound, map[string]counters{dayOne: {Attempts: 1}, dayTwo: {Attempts: 2}})
	add("lt+a", "late_modifier", ResolutionDisabled, map[string]counters{dayTwo: {Attempts: 2}})
	// Even a perfect-looking 5/5 near-miss ratio from one day remains
	// evidence-only until it repeats on a second active day.
	add("rt+b", "fallback", ResolutionUnbound, map[string]counters{dayOne: {Attempts: 5}})
	add("rt+x", "selected", ResolutionBound, map[string]counters{dayOne: {Attempts: 5}})
	add("rt+x", "fallback", ResolutionUnbound, map[string]counters{dayTwo: {Attempts: 5}})
	add("lt+y", "left_stick_active,pointer_context,selected", ResolutionBound, map[string]counters{dayOne: {Attempts: 2}})
	add("lt+y", "fallback,left_stick_active,pointer_context", ResolutionUnbound, map[string]counters{dayOne: {Attempts: 1}, dayTwo: {Attempts: 3}})

	summary := summarize(state, 0)
	if len(summary.ChordCandidates) != 4 {
		t.Fatalf("chord candidates = %#v", summary.ChordCandidates)
	}
	var candidate ChordCandidateSummary
	for _, item := range summary.ChordCandidates {
		if item.Gesture == "lt+a" {
			candidate = item
		}
	}
	if candidate.Total != 10 || candidate.Selected != 5 || candidate.NearMiss != 5 ||
		candidate.Fallback != 3 || candidate.LateModifier != 2 || candidate.PriorityBlocked != 3 ||
		candidate.Unbound != 3 || candidate.Disabled != 2 || candidate.ActiveDays != 2 ||
		candidate.NearMissDays != 2 || candidate.LateModifierDays != 1 || candidate.EligibleTotal != 10 ||
		candidate.EligibleNearMiss != 5 || candidate.EligibleLateModifier != 2 || !candidate.Qualified {
		t.Fatalf("lt+a candidate = %#v", candidate)
	}
	var ambiguous ChordCandidateSummary
	for _, item := range summary.ChordCandidates {
		if item.Gesture == "lt+y" {
			ambiguous = item
		}
	}
	if ambiguous.Total != 6 || ambiguous.Selected != 2 || ambiguous.EligibleSelected != 0 || ambiguous.NearMiss != 4 || ambiguous.Ambiguous != 6 ||
		ambiguous.PointerContext != 6 || ambiguous.StickContext != 6 || ambiguous.EligibleTotal != 0 || ambiguous.Qualified {
		t.Fatalf("pointer/stick candidate = %#v", ambiguous)
	}
	recommendations := strings.Join(summary.Recommendations, "\n")
	for _, want := range []string{"lt+a", "editor.exe", "editor", "s-current", "5/10"} {
		if !strings.Contains(recommendations, want) {
			t.Fatalf("recommendations missing %q: %s", want, recommendations)
		}
	}
	if strings.Contains(recommendations, "rt+b") || strings.Contains(recommendations, "rt+x") || strings.Contains(recommendations, "lt+y") {
		t.Fatalf("single-day candidate signal should stay evidence-only: %s", recommendations)
	}
	text := FormatText(summary)
	for _, want := range []string{"总曝光 10", "原始选中 5、有效选中 5", "近失误 5/10", "慢按 2/10", "活跃 2 天", "样本不足，仅展示", "歧义 6：pointer 6、stick 6；有效分母 0", "原始选中 2、有效选中 0"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text evidence missing %q:\n%s", want, text)
		}
	}
	directory := t.TempDir()
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	rendered := htmlstd.UnescapeString(string(html))
	for _, want := range []string{"组合候选", "lt+a", "editor.exe", "s-current", "5 / 10（50%", "歧义（pointer / stick）", "有效分母", "原始 / 有效选中", "2 / 0", "有效样本不足，仅展示，不形成建议"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("HTML chord evidence missing %q", want)
		}
	}
}

func TestUnknownOrCoalescedAppsNeverTriggerDecisionRecommendations(t *testing.T) {
	state := emptyAggregate()
	state.Strategies = []StrategyDefinition{{ID: "s-current"}}
	dayOne, dayTwo := "2026-07-20", "2026-07-21"
	for _, app := range []string{"", otherAppBucket} {
		key := entryKey{
			Kind: EventChordProbe, StrategyID: "s-current", ForegroundApp: app,
			ActiveProfile: "editor", Gesture: "lt+a", PhysicalGesture: "lt+a",
			Resolution: ResolutionObserved, CandidateResolution: ResolutionUnbound, Flags: "fallback",
		}
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: 6, FirstUsedDay: dayOne, LastUsedDay: dayTwo,
			Daily: map[string]counters{dayOne: {Attempts: 3}, dayTwo: {Attempts: 3}},
		}
	}
	correctionKey := entryKey{
		Kind: EventTransition, StrategyID: "s-current", ForegroundApp: otherAppBucket,
		ActiveProfile: "editor", RelatedAction: "arrow_up", RelatedGesture: "dpad_up",
		Action: "arrow_down", Gesture: "dpad_down", Resolution: ResolutionObserved,
		IntervalBucket: "250_750ms",
	}
	state.Entries[correctionKey] = &aggregateEntry{
		Key: correctionKey, Attempts: 3, FirstUsedDay: dayOne, LastUsedDay: dayTwo,
		Daily: map[string]counters{dayOne: {Attempts: 2}, dayTwo: {Attempts: 1}},
	}
	otherKey := correctionKey
	otherKey.Action = "click_left"
	otherKey.Gesture = "a"
	otherKey.IntervalBucket = "2_10s"
	state.Entries[otherKey] = &aggregateEntry{
		Key: otherKey, Attempts: 7, FirstUsedDay: dayOne, LastUsedDay: dayTwo,
		Daily: map[string]counters{dayOne: {Attempts: 3}, dayTwo: {Attempts: 4}},
	}

	summary := summarize(state, 0)
	if len(summary.ChordCandidates) != 2 || len(summary.CorrectionPatterns) != 1 {
		t.Fatalf("unknown app evidence = %#v", summary)
	}
	for _, candidate := range summary.ChordCandidates {
		if candidate.AppKnown || !candidate.Sufficient || candidate.Qualified {
			t.Fatalf("unknown app candidate qualified = %#v", candidate)
		}
	}
	if pattern := summary.CorrectionPatterns[0]; pattern.AppKnown || pattern.Qualified || pattern.Total != 10 {
		t.Fatalf("coalesced app transition qualified = %#v", pattern)
	}
	if len(summary.Recommendations) != 0 {
		t.Fatalf("unknown/coalesced app produced recommendations: %#v", summary.Recommendations)
	}
	text := FormatText(summary)
	if !strings.Contains(text, "App 未知或已合并多个 App，仅展示，不形成建议") ||
		!strings.Contains(text, "历史数据 / 无法识别") || !strings.Contains(text, otherAppBucket) {
		t.Fatalf("unknown app evidence not explicit:\n%s", text)
	}
	directory := t.TempDir()
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(htmlstd.UnescapeString(string(html)), "App 未知或已合并多个 App，仅展示，不形成建议") {
		t.Fatal("HTML did not explain the unknown/coalesced App decision gate")
	}
}

func TestPhysicalOverlapIsAnAttemptCountClueWithoutRecommendation(t *testing.T) {
	state := emptyAggregate()
	state.Strategies = []StrategyDefinition{{ID: "s-current"}}
	day := "2026-07-21"
	add := func(control, gesture, physical, action, flags string) {
		key := entryKey{
			Kind: EventInputAttempt, StrategyID: "s-current", ForegroundApp: "game.exe",
			ActiveProfile: "game", Control: control, Gesture: gesture, PhysicalGesture: physical,
			Action: action, Resolution: ResolutionBound, Flags: flags,
		}
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: 1, Successes: 1, FirstUsedDay: day, LastUsedDay: day,
			Daily: map[string]counters{day: {Attempts: 1, Successes: 1}},
		}
	}
	// One actual poll frame can produce one rising attempt for each button. The
	// persisted aggregate intentionally reports both attempts instead of
	// pretending it can reconstruct a frame ID.
	add("a", "a", "a+b", "click_left", "simultaneous_buttons")
	add("b", "b", "a+b", "escape", "simultaneous_buttons")
	add("x", "x", "x", "copy", "")

	summary := summarize(state, 0)
	if summary.TotalAttempts != 3 || len(summary.PhysicalOverlaps) != 1 {
		t.Fatalf("overlap summary = %#v", summary)
	}
	overlap := summary.PhysicalOverlaps[0]
	if overlap.PhysicalGesture != "a+b" || overlap.InputAttempts != 2 || overlap.ActiveDays != 1 ||
		overlap.ForegroundApp != "game.exe" || overlap.ActiveProfile != "game" || overlap.StrategyID != "s-current" {
		t.Fatalf("physical overlap = %#v", overlap)
	}
	text := FormatText(summary)
	for _, want := range []string{"数字键重叠 a+b", "2 条 input_attempt", "一个实际帧可能产生多条", "只作为线索，不直接推荐"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text overlap evidence missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(strings.Join(summary.Recommendations, "\n"), "a+b") {
		t.Fatalf("physical overlap must not directly recommend a remap: %#v", summary.Recommendations)
	}
	directory := t.TempDir()
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	rendered := htmlstd.UnescapeString(string(html))
	for _, want := range []string{"同帧数字键重叠线索", "a+b", "game.exe", "s-current", "input_attempt 条数", "无法按帧去重", "只作为线索"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("HTML overlap evidence missing %q", want)
		}
	}
}

func TestMappingAndComboReportsKeepStrategyVersionsSeparate(t *testing.T) {
	state := emptyAggregate()
	state.Strategies = []StrategyDefinition{{ID: "s-old"}, {ID: "s-new"}}
	day := "2026-07-21"
	for _, strategy := range []string{"s-old", "s-new"} {
		key := entryKey{
			Kind: EventInputAttempt, StrategyID: strategy, ForegroundApp: "editor.exe",
			ActiveProfile: "editor", BindingProfile: "default", Control: "a",
			Gesture: "lt+a", PhysicalGesture: "lt+a", GestureKind: GestureTriggerChord,
			Action: "enter", Resolution: ResolutionBound,
		}
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: 1, Successes: 1, FirstUsedDay: day, LastUsedDay: day,
			Daily: map[string]counters{day: {Attempts: 1, Successes: 1}},
		}
	}
	summary := summarize(state, 0)
	if len(summary.ComboGestures) != 2 || len(summary.Mappings) != 2 {
		t.Fatalf("strategy-separated combo/mapping = %#v / %#v", summary.ComboGestures, summary.Mappings)
	}
	seen := make(map[string]bool)
	for _, item := range summary.ComboGestures {
		seen[item.StrategyID] = true
	}
	if !seen["s-old"] || !seen["s-new"] {
		t.Fatalf("combo strategy IDs = %#v", summary.ComboGestures)
	}
	text := FormatText(summary)
	for _, want := range []string{"映射执行明细（按策略版本区分）", "策略 s-old", "策略 s-new"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text strategy evidence missing %q:\n%s", want, text)
		}
	}
	directory := t.TempDir()
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	rendered := htmlstd.UnescapeString(string(html))
	for _, want := range []string{"组合键与状态手势", "映射执行明细", "s-old", "s-new", "策略"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("HTML strategy evidence missing %q", want)
		}
	}
}

func TestTransitionRecommendationsRequireDenominatorAndMultipleActiveDays(t *testing.T) {
	state := emptyAggregate()
	state.Strategies = []StrategyDefinition{{ID: "s-current"}}
	dayOne, dayTwo := "2026-07-20", "2026-07-21"
	add := func(from, fromGesture, to, toGesture, bucket string, daily map[string]counters) {
		key := entryKey{
			Kind: EventTransition, StrategyID: "s-current", ForegroundApp: "editor.exe",
			ActiveProfile: "editor", RelatedAction: from, RelatedGesture: fromGesture,
			Action: to, Gesture: toGesture,
			Resolution: ResolutionObserved, IntervalBucket: bucket,
		}
		entry := &aggregateEntry{Key: key, Daily: daily}
		for day, value := range daily {
			entry.Attempts = saturatingAdd(entry.Attempts, value.Attempts)
			if entry.FirstUsedDay == "" || day < entry.FirstUsedDay {
				entry.FirstUsedDay = day
			}
			if day > entry.LastUsedDay {
				entry.LastUsedDay = day
			}
		}
		state.Entries[key] = entry
	}
	add("arrow_up", "dpad_up", "arrow_down", "dpad_down", "250_750ms", map[string]counters{dayOne: {Attempts: 2}, dayTwo: {Attempts: 1}})
	add("arrow_up", "dpad_up", "click_left", "a", "2_10s", map[string]counters{dayOne: {Attempts: 3}, dayTwo: {Attempts: 4}})
	// The same action from another gesture is a separate denominator.
	add("arrow_up", "left_stick_up", "click_left", "a", "2_10s", map[string]counters{dayOne: {Attempts: 10}, dayTwo: {Attempts: 10}})
	add("page_down", "dpad_down", "page_down", "dpad_down", "under_250ms", map[string]counters{dayOne: {Attempts: 1}, dayTwo: {Attempts: 2}})
	add("page_down", "dpad_down", "click_left", "a", "2_10s", map[string]counters{dayOne: {Attempts: 4}, dayTwo: {Attempts: 3}})
	add("tab_previous", "lb", "tab_next", "rb", "250_750ms", map[string]counters{dayOne: {Attempts: 3}})
	add("tab_previous", "lb", "click_left", "a", "2_10s", map[string]counters{dayOne: {Attempts: 3}, dayTwo: {Attempts: 4}})

	summary := summarize(state, 0)
	if len(summary.CorrectionPatterns) != 2 || len(summary.RepeatPatterns) != 1 {
		t.Fatalf("transition patterns = corrections %#v repeats %#v", summary.CorrectionPatterns, summary.RepeatPatterns)
	}
	var qualified, insufficient TransitionPatternSummary
	for _, item := range summary.CorrectionPatterns {
		if item.FromAction == "arrow_up" {
			qualified = item
		} else if item.FromAction == "tab_previous" {
			insufficient = item
		}
	}
	if qualified.Suspected != 3 || qualified.Total != 10 || qualified.ActiveDays != 2 ||
		qualified.FromGesture != "dpad_up" || qualified.ToGesture != "dpad_down" || !qualified.Qualified {
		t.Fatalf("qualified correction = %#v", qualified)
	}
	if insufficient.Total != 10 || insufficient.ActiveDays != 1 || insufficient.Qualified {
		t.Fatalf("single-day correction = %#v", insufficient)
	}
	if repeat := summary.RepeatPatterns[0]; repeat.Suspected != 3 || repeat.Total != 10 || repeat.ActiveDays != 2 || !repeat.Qualified {
		t.Fatalf("repeat pattern = %#v", repeat)
	}
	recommendations := strings.Join(summary.Recommendations, "\n")
	for _, want := range []string{"arrow_up", "arrow_down", "page_down", "3/10", "editor.exe", "editor", "s-current", "2 个活跃日"} {
		if !strings.Contains(recommendations, want) {
			t.Fatalf("recommendations missing %q: %s", want, recommendations)
		}
	}
	if strings.Contains(recommendations, "tab_previous") {
		t.Fatalf("single-day pattern must not trigger a recommendation: %s", recommendations)
	}
	text := FormatText(summary)
	for _, want := range []string{"疑似修正 arrow_up → arrow_down", "手势 dpad_up → dpad_down", "3/10（30%）", "快速重复 page_down", "证据活跃 2 天", "样本不足，仅展示，不形成建议"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text transition evidence missing %q:\n%s", want, text)
		}
	}
	directory := t.TempDir()
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	rendered := htmlstd.UnescapeString(string(html))
	for _, want := range []string{"疑似修正比率", "快速重复比率", "dpad_up", "dpad_down", "editor.exe", "s-current", "3 / 10（30%）", "样本或稳定性不足，仅展示，不形成建议"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("HTML transition evidence missing %q", want)
		}
	}
}

func TestProcessAndContextEvidenceExposeTheirDecisionContext(t *testing.T) {
	summary := Summary{
		Holds: []TraceEvidenceSummary{{
			StrategyID: "s-hold", ForegroundApp: "hold.exe", ActiveProfile: "hold-profile",
			PhysicalGesture: "lt", RelatedAction: "window_next", DurationBucket: "1_2s",
			CountBucket: "2", Reason: "released", CountSummary: CountSummary{Attempts: 2},
		}},
		ComposeSessions: []TraceEvidenceSummary{{
			StrategyID: "s-compose", ForegroundApp: "compose.exe", ActiveProfile: "compose-profile",
			Gesture: "voice", Action: "voice", DurationBucket: "3_9s", CountBucket: "3_4",
			Reason: "submit_succeeded", CountSummary: CountSummary{Attempts: 3, Successes: 1, Failures: 1},
		}},
		WindowSessions: []TraceEvidenceSummary{{
			StrategyID: "s-window", ForegroundApp: "window.exe", ActiveProfile: "window-profile",
			Gesture: "lt+lb|lt+rb", Action: "window_cycle", DurationBucket: "300_999ms",
			CountBucket: "gte_5", Reason: "released", CountSummary: CountSummary{Attempts: 5, Successes: 4, Failures: 1},
		}},
		RepeatEpisodes: []TraceEvidenceSummary{{
			StrategyID: "s-repeat", ForegroundApp: "repeat.exe", ActiveProfile: "repeat-profile",
			PhysicalGesture: "dpad_down", Gesture: "dpad_down", Action: "arrow_down",
			DurationBucket: "1_2s", CountBucket: "3_4", Reason: "released",
			CountSummary: CountSummary{Attempts: 4, Successes: 3, Failures: 1},
		}},
		ContextSessions: []TraceEvidenceSummary{{
			StrategyID: "s-context", ForegroundApp: "context.exe", ActiveProfile: "context-profile",
			Reason: "idle", CountSummary: CountSummary{Attempts: 6},
		}},
	}
	text := FormatText(summary)
	for _, want := range []string{
		"Hold lt：2 次（App hold.exe，场景 hold-profile，策略 s-hold",
		"Compose voice → voice：3 次（App compose.exe，场景 compose-profile，策略 s-compose",
		"窗口切换 lt+lb|lt+rb → window_cycle：5 次（App window.exe，场景 window-profile，策略 s-window",
		"连续操作 dpad_down / arrow_down：4 个 episode（App repeat.exe，场景 repeat-profile，策略 s-repeat",
		"结果 成功 1 / 失败 1 / 无结果 1",
		"交互会话（App context.exe，场景 context-profile，策略 s-context）：6 次（原因 idle）",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text process evidence missing %q:\n%s", want, text)
		}
	}

	directory := t.TempDir()
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	rendered := htmlstd.UnescapeString(string(html))
	for _, want := range []string{
		"过程证据", "结果：成功 / 失败 / 无结果", "Hold", "Compose", "Window", "Repeat",
		"hold.exe", "compose-profile", "s-window", "dpad_down", "3_4", "submit_succeeded",
		"1 / 1 / 1", "App 交互会话", "context.exe", "context-profile", "s-context", "会话次数", "idle",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("HTML process evidence missing %q", want)
		}
	}
}

func TestDiagnosticEvidenceDoesNotInflateInputDenominator(t *testing.T) {
	day := "2026-07-21"
	state := emptyAggregate()
	add := func(key entryKey, attempts uint64) {
		state.Entries[key] = &aggregateEntry{
			Key: key, Attempts: attempts, FirstUsedDay: day, LastUsedDay: day,
			Daily: map[string]counters{day: {Attempts: attempts}},
		}
	}
	add(entryKey{Kind: EventInputAttempt, StrategyID: "s-one", ForegroundApp: "app.exe", Control: "a", Gesture: "a", Action: "click_left", Resolution: ResolutionBound}, 1)
	add(entryKey{Kind: EventTransition, StrategyID: "s-one", ForegroundApp: "app.exe", Action: "arrow_down", RelatedAction: "arrow_up", Resolution: ResolutionObserved, IntervalBucket: "250_750ms"}, 7)
	add(entryKey{Kind: EventHoldEpisode, StrategyID: "s-one", ForegroundApp: "app.exe", Control: "a", Resolution: ResolutionObserved, DurationBucket: "1_2s"}, 3)
	summary := summarize(state, 0)
	if summary.TotalAttempts != 1 || len(summary.Transitions) != 1 || len(summary.Holds) != 1 {
		t.Fatalf("summary denominator/evidence = %#v", summary)
	}
}
