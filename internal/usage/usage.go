package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	legacySchemaVersion         = 1
	schemaVersion               = 2
	defaultFlushPeriod          = 10 * time.Second
	defaultMaxBatchSize         = 64
	snapshotRefreshTime         = time.Minute
	walCompactSize              = 256 * 1024
	walMaximumSize              = 1024 * 1024
	recentDayCount              = 90
	maximumAppBuckets           = 64
	maximumStrategies           = 8
	maximumEntries              = 8192
	maximumDailyCells           = 32768
	maximumSnapshotBytes        = 8 * 1024 * 1024
	maximumAppNameBytes         = 128
	maximumDimensionBytes       = 128
	maximumInventoryDefinitions = 512
	otherAppBucket              = "__other__"
)

// ErrDirectoryInUse indicates that another recorder owns the same local store.
// The operating-system lock is released automatically if that process exits.
var ErrDirectoryInUse = errors.New("local usage directory is already in use")

const (
	snapshotFileName = "usage-v1.snapshot.json"
	walFileName      = "usage-v1.wal.jsonl"
)

// Resolution describes how an observed control was interpreted.
type Resolution string

const (
	ResolutionBound    Resolution = "bound"
	ResolutionDisabled Resolution = "disabled"
	ResolutionUnbound  Resolution = "unbound"
	ResolutionObserved Resolution = "observed"
	ResolutionSystem   Resolution = "system"
)

// Outcome describes whether a resolved action completed successfully.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeNone    Outcome = "none"
)

// EventKind keeps the user-input denominator separate from diagnostic facts.
// Diagnostic events must never be added to input-attempt totals.
type EventKind string

const (
	// EventLegacy is used by observations written before tracing was added.
	EventLegacy             EventKind = ""
	EventInputAttempt       EventKind = "input_attempt"
	EventPhysicalActivation EventKind = "physical_activation"
	EventChordProbe         EventKind = "chord_probe"
	EventHoldEpisode        EventKind = "hold_episode"
	EventTransition         EventKind = "transition"
	EventComposeSession     EventKind = "compose_session"
	EventWindowSession      EventKind = "window_session"
	EventContextSession     EventKind = "context_session"
	EventRepeatEpisode      EventKind = "repeat_episode"
)

// GestureKind distinguishes simultaneous chords from stateful sequences and
// ordinary buttons. A plus sign alone is not sufficient: voice+a and lt+rb
// have very different ergonomics.
type GestureKind string

const (
	GestureSingle           GestureKind = "single"
	GestureTriggerChord     GestureKind = "trigger_chord"
	GestureModeSequence     GestureKind = "mode_sequence"
	GestureDigitalHold      GestureKind = "digital_hold"
	GestureTriggerHold      GestureKind = "trigger_hold"
	GestureAnalogActivation GestureKind = "analog_activation"
	GestureSystemHold       GestureKind = "system_hold"
)

// Observation is one intentional, already-normalized controller interaction.
// ForegroundApp may contain only an executable base name. Observations must not
// contain raw keyboard input, window titles, full process paths, or pointer data.
type Observation struct {
	At                  time.Time   `json:"-"`
	Sequence            uint64      `json:"-"`
	Kind                EventKind   `json:"kind,omitempty"`
	StrategyID          string      `json:"strategy_id,omitempty"`
	ForegroundApp       string      `json:"foreground_app,omitempty"`
	ActiveProfile       string      `json:"active_profile,omitempty"`
	BindingProfile      string      `json:"binding_profile,omitempty"`
	Control             string      `json:"control,omitempty"`
	Gesture             string      `json:"gesture,omitempty"`
	PhysicalGesture     string      `json:"physical_gesture,omitempty"`
	GestureKind         GestureKind `json:"gesture_kind,omitempty"`
	Action              string      `json:"action,omitempty"`
	RelatedGesture      string      `json:"related_gesture,omitempty"`
	RelatedAction       string      `json:"related_action,omitempty"`
	Resolution          Resolution  `json:"resolution"`
	CandidateResolution Resolution  `json:"candidate_resolution,omitempty"`
	Outcome             Outcome     `json:"outcome"`
	IntervalBucket      string      `json:"interval_bucket,omitempty"`
	DurationBucket      string      `json:"duration_bucket,omitempty"`
	CountBucket         string      `json:"count_bucket,omitempty"`
	Reason              string      `json:"reason,omitempty"`
	Flags               string      `json:"flags,omitempty"`
}

// BindingDefinition captures the current effective binding inventory. It is
// stored with the aggregate so readers can distinguish disabled mappings from
// mappings that have never been used.
type BindingDefinition struct {
	Profile    string     `json:"profile"`
	Gesture    string     `json:"gesture"`
	Action     string     `json:"action,omitempty"`
	Resolution Resolution `json:"resolution"`
}

// StrategyDefinition preserves the binding inventory that produced one set of
// observations. Without it, data from before and after a remap is inseparable.
type StrategyDefinition struct {
	ID        string              `json:"id"`
	Inventory []BindingDefinition `json:"inventory"`
}

// StrategyID returns a stable, privacy-safe revision for the effective binding
// inventory and the small set of settings that change gesture semantics.
func StrategyID(inventory []BindingDefinition, semantics map[string]string) string {
	definitions := cloneInventory(inventory)
	sort.Slice(definitions, func(left, right int) bool {
		leftValue, rightValue := definitions[left], definitions[right]
		if leftValue.Profile != rightValue.Profile {
			return leftValue.Profile < rightValue.Profile
		}
		if leftValue.Gesture != rightValue.Gesture {
			return leftValue.Gesture < rightValue.Gesture
		}
		if leftValue.Action != rightValue.Action {
			return leftValue.Action < rightValue.Action
		}
		return leftValue.Resolution < rightValue.Resolution
	})
	type semanticValue struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	values := make([]semanticValue, 0, len(semantics))
	for name, value := range semantics {
		values = append(values, semanticValue{Name: name, Value: value})
	}
	sort.Slice(values, func(left, right int) bool { return values[left].Name < values[right].Name })
	payload, _ := json.Marshal(struct {
		Inventory []BindingDefinition `json:"inventory"`
		Semantics []semanticValue     `json:"semantics"`
	}{Inventory: definitions, Semantics: values})
	digest := sha256.Sum256(payload)
	return "s-" + hex.EncodeToString(digest[:8])
}

// Options configures a local file recorder.
type Options struct {
	Directory     string
	Inventory     []BindingDefinition
	StrategyID    string
	Controls      []string
	Now           func() time.Time
	FlushInterval time.Duration
	MaxBatchSize  int
	OnError       func(error)
}

// Recorder accepts usage observations without blocking the input loop.
type Recorder interface {
	Record(Observation)
	Close() error
}

// FileRecorder aggregates observations on a worker goroutine and persists
// batches to a write-ahead log. Record never performs filesystem I/O.
type FileRecorder struct {
	directory     string
	now           func() time.Time
	flushInterval time.Duration
	maxBatchSize  int
	onError       func(error)

	records chan Observation
	stop    chan struct{}
	done    chan struct{}

	stateMu  sync.RWMutex
	closed   bool
	closeMu  sync.Once
	dropped  atomic.Uint64
	sequence atomic.Uint64

	closeErr    error
	state       aggregateState
	strategyID  string
	storeUnlock func() error

	lastSnapshotRefresh time.Time
}

type entryKey struct {
	Kind                EventKind
	StrategyID          string
	ForegroundApp       string
	ActiveProfile       string
	BindingProfile      string
	Control             string
	Gesture             string
	PhysicalGesture     string
	GestureKind         GestureKind
	Action              string
	RelatedGesture      string
	RelatedAction       string
	Resolution          Resolution
	CandidateResolution Resolution
	IntervalBucket      string
	DurationBucket      string
	CountBucket         string
	Reason              string
	Flags               string
}

type deltaKey struct {
	Entry entryKey
	Day   string
}

type counters struct {
	Attempts  uint64 `json:"attempts"`
	Successes uint64 `json:"successes"`
	Failures  uint64 `json:"failures"`
}

type aggregateEntry struct {
	Key          entryKey
	Attempts     uint64
	Successes    uint64
	Failures     uint64
	FirstUsedDay string
	LastUsedDay  string
	Daily        map[string]counters
}

type aggregateState struct {
	UpdatedDay     string
	AppliedThrough uint64
	Inventory      []BindingDefinition
	Strategies     []StrategyDefinition
	Controls       []string
	Entries        map[entryKey]*aggregateEntry
	Dropped        uint64
	InvalidDropped uint64
	Coalesced      uint64
	SequenceBreaks uint64
}

type healthDelta struct {
	InvalidDropped uint64
	Coalesced      uint64
	SequenceBreaks uint64
}

type tracePoint struct {
	At            time.Time
	StrategyID    string
	ForegroundApp string
	ActiveProfile string
	Gesture       string
	Action        string
}

type traceProcessor struct {
	state        *aggregateState
	apps         map[string]string
	lastSequence uint64
	lastInput    *tracePoint
	lastActivity *tracePoint
	health       healthDelta
}

// SnapshotPath returns the aggregate snapshot path for dir.
func SnapshotPath(dir string) string {
	return filepath.Join(dir, snapshotFileName)
}

// WALPath returns the write-ahead log path for dir.
func WALPath(dir string) string {
	return filepath.Join(dir, walFileName)
}

// Open loads any existing snapshot, replays complete WAL batches, and starts a
// non-blocking recorder. A partial final WAL line left by a crash is discarded.
func Open(options Options) (*FileRecorder, error) {
	if options.Directory == "" {
		return nil, errors.New("usage directory cannot be empty")
	}
	if options.FlushInterval < 0 {
		return nil, errors.New("usage flush interval cannot be negative")
	}
	if options.MaxBatchSize < 0 {
		return nil, errors.New("usage max batch size cannot be negative")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.FlushInterval == 0 {
		options.FlushInterval = defaultFlushPeriod
	}
	if options.MaxBatchSize == 0 {
		options.MaxBatchSize = defaultMaxBatchSize
	}
	if options.OnError == nil {
		options.OnError = func(error) {}
	}
	if len(options.Inventory) > maximumInventoryDefinitions {
		return nil, fmt.Errorf("usage inventory has %d definitions; maximum is %d", len(options.Inventory), maximumInventoryDefinitions)
	}
	for index, definition := range options.Inventory {
		if !validResolution(definition.Resolution) {
			return nil, fmt.Errorf("usage inventory[%d] has invalid resolution %q", index, definition.Resolution)
		}
		for _, field := range []struct {
			name  string
			value string
		}{
			{name: "profile", value: definition.Profile},
			{name: "gesture", value: definition.Gesture},
			{name: "action", value: definition.Action},
		} {
			if err := validateDimension("inventory "+field.name, field.value); err != nil {
				return nil, fmt.Errorf("usage inventory[%d]: %w", index, err)
			}
		}
	}
	if options.StrategyID == "" && len(options.Inventory) > 0 {
		options.StrategyID = StrategyID(options.Inventory, nil)
	}

	if err := prepareDirectory(options.Directory); err != nil {
		return nil, err
	}
	storeUnlock, err := acquireStoreLock(options.Directory)
	if err != nil {
		return nil, err
	}
	releaseStoreOnError := true
	defer func() {
		if releaseStoreOnError {
			_ = storeUnlock()
		}
	}()
	state, recoveryErr, err := loadAggregate(options.Directory)
	if err != nil {
		return nil, err
	}
	if recoveryErr != nil {
		safeReport(options.OnError, recoveryErr)
	}
	if err := replayWAL(options.Directory, &state); err != nil {
		return nil, err
	}
	openedAt := options.Now()
	state.Inventory = cloneInventory(options.Inventory)
	state.Controls = cloneStrings(options.Controls)
	pruneDaily(&state, openedAt)
	ensureStrategy(&state, options.StrategyID, options.Inventory)
	enforceAggregateLimits(&state)
	if err := ensureWAL(options.Directory); err != nil {
		return nil, err
	}
	// Persist the current inventory immediately, even before the first input.
	// This lets readers distinguish "never used" from "not inventoried" and
	// also folds any replayed WAL into a clean crash-safe snapshot.
	if err := compact(options.Directory, &state, openedAt); err != nil {
		return nil, err
	}
	refreshReportBestEffort(options.Directory, options.OnError)

	queueSize := options.MaxBatchSize * 4
	if queueSize < defaultMaxBatchSize {
		queueSize = defaultMaxBatchSize
	}
	recorder := &FileRecorder{
		directory:           options.Directory,
		now:                 options.Now,
		flushInterval:       options.FlushInterval,
		maxBatchSize:        options.MaxBatchSize,
		onError:             options.OnError,
		records:             make(chan Observation, queueSize),
		stop:                make(chan struct{}),
		done:                make(chan struct{}),
		state:               state,
		strategyID:          options.StrategyID,
		storeUnlock:         storeUnlock,
		lastSnapshotRefresh: openedAt,
	}
	go recorder.run()
	releaseStoreOnError = false
	return recorder, nil
}

// Record queues observation if capacity is available. When the queue is full,
// the observation is dropped and accounted for in the next durable batch.
func (r *FileRecorder) Record(observation Observation) {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	if r.closed {
		return
	}
	observation.Sequence = r.sequence.Add(1)
	select {
	case r.records <- observation:
	default:
		r.dropped.Add(1)
	}
}

// Close drains accepted observations, flushes them, and compacts the WAL into
// an atomic snapshot. It is safe to call Close more than once.
func (r *FileRecorder) Close() error {
	r.closeMu.Do(func() {
		r.stateMu.Lock()
		r.closed = true
		close(r.stop)
		r.stateMu.Unlock()
		<-r.done
		if r.storeUnlock != nil {
			if err := r.storeUnlock(); err != nil && r.closeErr == nil {
				r.closeErr = fmt.Errorf("release usage directory lock: %w", err)
			}
		}
	})
	return r.closeErr
}

func (r *FileRecorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()

	pending := make(map[deltaKey]counters)
	pendingCount := 0
	var pendingDropped uint64
	processor := newTraceProcessor(&r.state)

	flush := func() error {
		pendingDropped = saturatingAdd(pendingDropped, r.dropped.Swap(0))
		health := processor.takeHealth()
		if pendingCount == 0 && pendingDropped == 0 && health == (healthDelta{}) {
			return nil
		}
		batch := makeWALBatch(r.state.AppliedThrough+1, pending, pendingDropped)
		batch.InvalidDropped = health.InvalidDropped
		batch.Coalesced = health.Coalesced
		batch.SequenceBreaks = health.SequenceBreaks
		appendErr := appendWALBatch(r.directory, batch)
		if errors.Is(appendErr, errWALNeedsCompaction) {
			compactedAt := r.now()
			if err := compact(r.directory, &r.state, compactedAt); err != nil {
				appendErr = fmt.Errorf("compact full usage WAL: %w", err)
			} else {
				r.lastSnapshotRefresh = compactedAt
				appendErr = appendWALBatch(r.directory, batch)
			}
		}
		if appendErr != nil {
			processor.restoreHealth(health)
			return appendErr
		}
		applyWALBatch(&r.state, batch)
		clear(pending)
		pendingCount = 0
		pendingDropped = 0
		refreshReportBestEffort(r.directory, r.onError)
		return nil
	}

	for {
		select {
		case observation := <-r.records:
			if observation.StrategyID == "" {
				observation.StrategyID = r.strategyID
			}
			added, err := processor.add(pending, observation, r.now())
			if err != nil {
				r.report(err)
				continue
			}
			pendingCount += added
			if pendingCount >= r.maxBatchSize {
				if err := flush(); err != nil {
					r.report(err)
				} else if err := r.compactIfNeeded(r.now()); err != nil {
					r.report(err)
				}
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				r.report(err)
			} else if err := r.compactIfNeeded(r.now()); err != nil {
				r.report(err)
			}
		case <-r.stop:
			for {
				select {
				case observation := <-r.records:
					if observation.StrategyID == "" {
						observation.StrategyID = r.strategyID
					}
					added, err := processor.add(pending, observation, r.now())
					if err != nil {
						r.report(err)
						continue
					}
					pendingCount += added
				default:
					if err := flush(); err != nil {
						r.closeErr = err
						return
					}
					if err := compact(r.directory, &r.state, r.now()); err != nil {
						r.closeErr = err
						return
					}
					refreshReportBestEffort(r.directory, r.onError)
					return
				}
			}
		}
	}
}

func (r *FileRecorder) compactIfNeeded(now time.Time) error {
	refreshDue := !now.Before(r.lastSnapshotRefresh.Add(snapshotRefreshTime))
	compacted, err := maybeCompact(r.directory, &r.state, now, refreshDue)
	if err == nil && compacted {
		r.lastSnapshotRefresh = now
	}
	return err
}

func addPending(pending map[deltaKey]counters, observation Observation, now time.Time) error {
	if err := validateObservationDimensions(observation); err != nil {
		return err
	}
	if observation.Kind != EventLegacy && !validEventKind(observation.Kind) {
		return fmt.Errorf("record usage: invalid event kind %q", observation.Kind)
	}
	if !validResolution(observation.Resolution) {
		return fmt.Errorf("record usage: invalid resolution %q", observation.Resolution)
	}
	if observation.CandidateResolution != "" && !validResolution(observation.CandidateResolution) {
		return fmt.Errorf("record usage: invalid candidate resolution %q", observation.CandidateResolution)
	}
	if observation.GestureKind != "" && !validGestureKind(observation.GestureKind) {
		return fmt.Errorf("record usage: invalid gesture kind %q", observation.GestureKind)
	}
	if observation.Outcome == "" {
		observation.Outcome = OutcomeNone
	}
	if !validOutcome(observation.Outcome) {
		return fmt.Errorf("record usage: invalid outcome %q", observation.Outcome)
	}
	if observation.At.IsZero() {
		observation.At = now
	}
	key := deltaKey{
		Entry: entryKey{
			Kind:                observation.Kind,
			StrategyID:          observation.StrategyID,
			ForegroundApp:       observation.ForegroundApp,
			ActiveProfile:       observation.ActiveProfile,
			BindingProfile:      observation.BindingProfile,
			Control:             observation.Control,
			Gesture:             observation.Gesture,
			PhysicalGesture:     observation.PhysicalGesture,
			GestureKind:         observation.GestureKind,
			Action:              observation.Action,
			RelatedGesture:      observation.RelatedGesture,
			RelatedAction:       observation.RelatedAction,
			Resolution:          observation.Resolution,
			CandidateResolution: observation.CandidateResolution,
			IntervalBucket:      observation.IntervalBucket,
			DurationBucket:      observation.DurationBucket,
			CountBucket:         observation.CountBucket,
			Reason:              observation.Reason,
			Flags:               observation.Flags,
		},
		Day: dayString(observation.At),
	}
	value := pending[key]
	increment := counters{Attempts: 1}
	switch observation.Outcome {
	case OutcomeSuccess:
		increment.Successes = 1
	case OutcomeFailure:
		increment.Failures = 1
	}
	pending[key] = addCounters(value, increment)
	return nil
}

func validateObservationDimensions(observation Observation) error {
	values := []struct {
		name  string
		value string
	}{
		{"strategy_id", observation.StrategyID},
		{"foreground_app", observation.ForegroundApp},
		{"active_profile", observation.ActiveProfile},
		{"binding_profile", observation.BindingProfile},
		{"control", observation.Control},
		{"gesture", observation.Gesture},
		{"physical_gesture", observation.PhysicalGesture},
		{"action", observation.Action},
		{"related_gesture", observation.RelatedGesture},
		{"related_action", observation.RelatedAction},
		{"interval_bucket", observation.IntervalBucket},
		{"duration_bucket", observation.DurationBucket},
		{"count_bucket", observation.CountBucket},
		{"reason", observation.Reason},
		{"flags", observation.Flags},
	}
	for _, item := range values {
		if err := validateDimension(item.name, item.value); err != nil {
			return err
		}
	}
	return nil
}

func validateDimension(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("record usage: %s is not valid UTF-8", name)
	}
	if len([]byte(value)) > maximumDimensionBytes {
		return fmt.Errorf("record usage: %s exceeds %d UTF-8 bytes", name, maximumDimensionBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("record usage: %s contains a control character", name)
		}
	}
	return nil
}

func newTraceProcessor(state *aggregateState) *traceProcessor {
	processor := &traceProcessor{state: state, apps: make(map[string]string)}
	for key := range state.Entries {
		if key.ForegroundApp == "" || key.ForegroundApp == otherAppBucket {
			continue
		}
		processor.apps[strings.ToLower(key.ForegroundApp)] = key.ForegroundApp
	}
	return processor
}

func (p *traceProcessor) add(pending map[deltaKey]counters, observation Observation, now time.Time) (int, error) {
	if observation.At.IsZero() {
		observation.At = now
	}
	app, coalesced, err := p.appBucket(observation.ForegroundApp)
	if err != nil {
		p.health.InvalidDropped = saturatingAdd(p.health.InvalidDropped, 1)
		p.lastInput = nil
		p.lastActivity = nil
		return 0, err
	}
	observation.ForegroundApp = app
	if coalesced {
		p.health.Coalesced = saturatingAdd(p.health.Coalesced, 1)
	}
	if observation.Sequence != 0 {
		if p.lastSequence != 0 && observation.Sequence != p.lastSequence+1 {
			p.lastInput = nil
			p.lastActivity = nil
			p.health.SequenceBreaks = saturatingAdd(p.health.SequenceBreaks, 1)
		}
		p.lastSequence = observation.Sequence
	}

	derived := p.derive(observation)
	items := make([]Observation, 0, len(derived)+1)
	items = append(items, observation)
	items = append(items, derived...)
	added := 0
	for _, item := range items {
		if err := p.addBounded(pending, item, now); err != nil {
			p.health.InvalidDropped = saturatingAdd(p.health.InvalidDropped, 1)
			p.lastInput = nil
			p.lastActivity = nil
			return added, err
		}
		added++
	}
	return added, nil
}

func (p *traceProcessor) derive(observation Observation) []Observation {
	if observation.Kind != EventInputAttempt && observation.Kind != EventPhysicalActivation {
		return nil
	}
	current := tracePoint{
		At:            observation.At,
		StrategyID:    observation.StrategyID,
		ForegroundApp: observation.ForegroundApp,
		ActiveProfile: observation.ActiveProfile,
		Gesture:       observation.Gesture,
		Action:        observation.Action,
	}
	var result []Observation
	newSessionReason := ""
	if p.lastActivity == nil {
		newSessionReason = "start"
	} else {
		gap := current.At.Sub(p.lastActivity.At)
		sameContext := current.StrategyID == p.lastActivity.StrategyID &&
			strings.EqualFold(current.ForegroundApp, p.lastActivity.ForegroundApp) &&
			current.ActiveProfile == p.lastActivity.ActiveProfile
		if !sameContext {
			newSessionReason = "context_changed"
		} else if gap < 0 {
			p.lastInput = nil
			p.health.SequenceBreaks = saturatingAdd(p.health.SequenceBreaks, 1)
		} else if gap >= 60*time.Second {
			newSessionReason = "idle"
		}
	}
	if newSessionReason != "" {
		result = append(result, Observation{
			At:            current.At,
			Kind:          EventContextSession,
			StrategyID:    current.StrategyID,
			ForegroundApp: current.ForegroundApp,
			ActiveProfile: current.ActiveProfile,
			Resolution:    ResolutionObserved,
			Outcome:       OutcomeNone,
			Reason:        newSessionReason,
		})
	}
	p.lastActivity = &current
	if observation.Kind != EventInputAttempt {
		// Trigger rising edges are modifiers for the input attempt that follows,
		// so clearing here would systematically erase action-to-chord
		// transitions. Analog stick activation is independent activity and still
		// breaks the action chain.
		if observation.Control != "lt" && observation.Control != "rt" {
			p.lastInput = nil
		}
		return result
	}
	if hasTraceFlag(observation.Flags, "simultaneous_buttons") {
		// Multiple rising buttons from one controller frame have no reliable
		// ordering. Do not connect the arbitrary first iterated button to the
		// preceding action, and do not use either edge as the next origin.
		p.lastInput = nil
		return result
	}
	if observation.Outcome != OutcomeSuccess {
		// A failed or merely observed dispatch cannot be treated as an action
		// the user then corrected or repeated. It remains stored as an input
		// attempt and still contributes to the context session, but it breaks
		// the ordered action chain in both directions.
		p.lastInput = nil
		return result
	}
	if p.lastInput != nil {
		gap := current.At.Sub(p.lastInput.At)
		sameContext := current.StrategyID == p.lastInput.StrategyID &&
			strings.EqualFold(current.ForegroundApp, p.lastInput.ForegroundApp) &&
			current.ActiveProfile == p.lastInput.ActiveProfile
		switch {
		case gap == 0:
			// Several rising edges from one polled controller frame have no
			// reliable order. Do not invent a sequence from loop iteration.
			p.lastInput = nil
			return result
		case gap < 0:
			p.lastInput = nil
			p.health.SequenceBreaks = saturatingAdd(p.health.SequenceBreaks, 1)
			return result
		case sameContext && gap <= 10*time.Second && p.lastInput.Action != "" && current.Action != "":
			result = append(result, Observation{
				At:             current.At,
				Kind:           EventTransition,
				StrategyID:     current.StrategyID,
				ForegroundApp:  current.ForegroundApp,
				ActiveProfile:  current.ActiveProfile,
				Gesture:        current.Gesture,
				Action:         current.Action,
				RelatedGesture: p.lastInput.Gesture,
				RelatedAction:  p.lastInput.Action,
				Resolution:     ResolutionObserved,
				Outcome:        OutcomeNone,
				IntervalBucket: transitionBucket(gap),
			})
		}
	}
	p.lastInput = &current
	return result
}

func (p *traceProcessor) addBounded(pending map[deltaKey]counters, observation Observation, now time.Time) error {
	key, err := observationDeltaKey(observation, now)
	if err != nil {
		return err
	}
	const overflowReserve = 4
	if !p.entryExists(pending, key.Entry) && p.entryCount(pending) >= maximumEntries-overflowReserve {
		observation = overflowObservation(observation)
		key, err = observationDeltaKey(observation, now)
		if err != nil {
			return err
		}
		p.health.Coalesced = saturatingAdd(p.health.Coalesced, 1)
	}
	value := pending[key]
	increment := counters{Attempts: 1}
	switch observation.Outcome {
	case OutcomeSuccess:
		increment.Successes = 1
	case OutcomeFailure:
		increment.Failures = 1
	}
	pending[key] = addCounters(value, increment)
	return nil
}

func observationDeltaKey(observation Observation, now time.Time) (deltaKey, error) {
	temporary := make(map[deltaKey]counters, 1)
	if err := addPending(temporary, observation, now); err != nil {
		return deltaKey{}, err
	}
	for key := range temporary {
		return key, nil
	}
	return deltaKey{}, errors.New("record usage: observation produced no aggregate key")
}

func (p *traceProcessor) entryExists(pending map[deltaKey]counters, key entryKey) bool {
	if _, found := p.state.Entries[key]; found {
		return true
	}
	for pendingKey := range pending {
		if pendingKey.Entry == key {
			return true
		}
	}
	return false
}

func (p *traceProcessor) entryCount(pending map[deltaKey]counters) int {
	count := len(p.state.Entries)
	seen := make(map[entryKey]struct{})
	for key := range pending {
		if _, found := p.state.Entries[key.Entry]; found {
			continue
		}
		seen[key.Entry] = struct{}{}
	}
	return count + len(seen)
}

func (p *traceProcessor) appBucket(value string) (string, bool, error) {
	value, err := sanitizeAppName(value)
	if err != nil || value == "" || value == otherAppBucket {
		return value, false, err
	}
	canonical := strings.ToLower(value)
	if existing, found := p.apps[canonical]; found {
		return existing, false, nil
	}
	if len(p.apps) >= maximumAppBuckets {
		return otherAppBucket, true, nil
	}
	p.apps[canonical] = value
	return value, false, nil
}

func (p *traceProcessor) takeHealth() healthDelta {
	value := p.health
	p.health = healthDelta{}
	return value
}

func (p *traceProcessor) restoreHealth(value healthDelta) {
	p.health.InvalidDropped = saturatingAdd(p.health.InvalidDropped, value.InvalidDropped)
	p.health.Coalesced = saturatingAdd(p.health.Coalesced, value.Coalesced)
	p.health.SequenceBreaks = saturatingAdd(p.health.SequenceBreaks, value.SequenceBreaks)
}

func sanitizeAppName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if index := strings.LastIndexAny(value, `/\\`); index >= 0 {
		value = value[index+1:]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !utf8.ValidString(value) {
		return "", errors.New("record usage: foreground app is not valid UTF-8")
	}
	if len([]byte(value)) > maximumAppNameBytes {
		return "", fmt.Errorf("record usage: foreground app exceeds %d UTF-8 bytes", maximumAppNameBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New("record usage: foreground app contains a control character")
		}
	}
	return value, nil
}

func transitionBucket(value time.Duration) string {
	switch {
	case value < 250*time.Millisecond:
		return "under_250ms"
	case value < 750*time.Millisecond:
		return "250_750ms"
	case value < 2*time.Second:
		return "750ms_2s"
	default:
		return "2_10s"
	}
}

func overflowObservation(observation Observation) Observation {
	kind := EventHoldEpisode
	outcome := OutcomeNone
	if observation.Kind == EventInputAttempt || observation.Kind == EventLegacy {
		kind = EventInputAttempt
		outcome = observation.Outcome
	}
	return Observation{
		At:            observation.At,
		Kind:          kind,
		ForegroundApp: otherAppBucket,
		Resolution:    ResolutionObserved,
		Outcome:       outcome,
		Reason:        "cardinality_limit",
	}
}

func overflowEntryKey(value entryKey) entryKey {
	kind := EventHoldEpisode
	if value.Kind == EventInputAttempt || value.Kind == EventLegacy {
		kind = EventInputAttempt
	}
	return entryKey{
		Kind:          kind,
		ForegroundApp: otherAppBucket,
		Resolution:    ResolutionObserved,
		Reason:        "cardinality_limit",
	}
}

func (r *FileRecorder) report(err error) {
	if err != nil {
		safeReport(r.onError, err)
	}
}

func safeReport(callback func(error), err error) {
	defer func() {
		_ = recover()
	}()
	callback(err)
}

func validResolution(value Resolution) bool {
	switch value {
	case ResolutionBound, ResolutionDisabled, ResolutionUnbound, ResolutionObserved, ResolutionSystem:
		return true
	default:
		return false
	}
}

func validOutcome(value Outcome) bool {
	switch value {
	case OutcomeSuccess, OutcomeFailure, OutcomeNone:
		return true
	default:
		return false
	}
}

func validEventKind(value EventKind) bool {
	switch value {
	case EventInputAttempt, EventPhysicalActivation, EventChordProbe,
		EventHoldEpisode, EventTransition, EventComposeSession,
		EventWindowSession, EventContextSession, EventRepeatEpisode:
		return true
	default:
		return false
	}
}

func validGestureKind(value GestureKind) bool {
	switch value {
	case GestureSingle, GestureTriggerChord, GestureModeSequence,
		GestureDigitalHold, GestureTriggerHold, GestureAnalogActivation,
		GestureSystemHold:
		return true
	default:
		return false
	}
}

func dayString(value time.Time) string {
	return value.Format("2006-01-02")
}

func saturatingAdd(left, right uint64) uint64 {
	result := left + right
	if result < left {
		return ^uint64(0)
	}
	return result
}

func addCounters(value, increment counters) counters {
	available := ^uint64(0) - value.Attempts
	attempts := increment.Attempts
	if attempts > available {
		attempts = available
	}
	successes := increment.Successes
	if successes > attempts {
		successes = attempts
	}
	failures := increment.Failures
	if failures > attempts-successes {
		failures = attempts - successes
	}
	value.Attempts += attempts
	value.Successes += successes
	value.Failures += failures
	return value
}

func cloneInventory(value []BindingDefinition) []BindingDefinition {
	result := make([]BindingDefinition, len(value))
	copy(result, value)
	return result
}

func cloneStrategies(value []StrategyDefinition) []StrategyDefinition {
	result := make([]StrategyDefinition, len(value))
	for index, strategy := range value {
		result[index] = StrategyDefinition{ID: strategy.ID, Inventory: cloneInventory(strategy.Inventory)}
	}
	return result
}

func ensureStrategy(state *aggregateState, id string, inventory []BindingDefinition) {
	if id == "" {
		return
	}
	for index := range state.Strategies {
		if state.Strategies[index].ID == id {
			updated := StrategyDefinition{ID: id, Inventory: cloneInventory(inventory)}
			copy(state.Strategies[index:], state.Strategies[index+1:])
			state.Strategies[len(state.Strategies)-1] = updated
			return
		}
	}
	state.Strategies = append(state.Strategies, StrategyDefinition{ID: id, Inventory: cloneInventory(inventory)})
	if len(state.Strategies) > maximumStrategies {
		state.Strategies = append([]StrategyDefinition(nil), state.Strategies[len(state.Strategies)-maximumStrategies:]...)
		state.Coalesced = saturatingAdd(state.Coalesced, 1)
	}
}

func cloneStrings(value []string) []string {
	result := make([]string, len(value))
	copy(result, value)
	return result
}
