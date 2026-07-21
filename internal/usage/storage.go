package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type snapshot struct {
	SchemaVersion  int                  `json:"schema_version"`
	UpdatedDay     string               `json:"updated_day"`
	AppliedThrough uint64               `json:"applied_through"`
	Inventory      []BindingDefinition  `json:"inventory"`
	Strategies     []StrategyDefinition `json:"strategies,omitempty"`
	Controls       []string             `json:"controls"`
	Entries        []snapshotEntry      `json:"entries"`
	Dropped        uint64               `json:"dropped"`
	InvalidDropped uint64               `json:"invalid_dropped,omitempty"`
	Coalesced      uint64               `json:"coalesced,omitempty"`
	SequenceBreaks uint64               `json:"sequence_breaks,omitempty"`
}

type snapshotEntry struct {
	Kind                EventKind           `json:"kind,omitempty"`
	StrategyID          string              `json:"strategy_id,omitempty"`
	ForegroundApp       string              `json:"foreground_app,omitempty"`
	ActiveProfile       string              `json:"active_profile,omitempty"`
	BindingProfile      string              `json:"binding_profile,omitempty"`
	Control             string              `json:"control,omitempty"`
	Gesture             string              `json:"gesture,omitempty"`
	PhysicalGesture     string              `json:"physical_gesture,omitempty"`
	GestureKind         GestureKind         `json:"gesture_kind,omitempty"`
	Action              string              `json:"action,omitempty"`
	RelatedGesture      string              `json:"related_gesture,omitempty"`
	RelatedAction       string              `json:"related_action,omitempty"`
	Resolution          Resolution          `json:"resolution"`
	CandidateResolution Resolution          `json:"candidate_resolution,omitempty"`
	IntervalBucket      string              `json:"interval_bucket,omitempty"`
	DurationBucket      string              `json:"duration_bucket,omitempty"`
	CountBucket         string              `json:"count_bucket,omitempty"`
	Reason              string              `json:"reason,omitempty"`
	Flags               string              `json:"flags,omitempty"`
	Attempts            uint64              `json:"attempts"`
	Successes           uint64              `json:"successes"`
	Failures            uint64              `json:"failures"`
	FirstUsedDay        string              `json:"first_used_day,omitempty"`
	LastUsedDay         string              `json:"last_used_day,omitempty"`
	Daily               map[string]counters `json:"daily"`
}

type walBatch struct {
	SchemaVersion  int        `json:"schema_version"`
	BatchID        uint64     `json:"batch_id"`
	Deltas         []walDelta `json:"deltas"`
	Dropped        uint64     `json:"dropped"`
	InvalidDropped uint64     `json:"invalid_dropped,omitempty"`
	Coalesced      uint64     `json:"coalesced,omitempty"`
	SequenceBreaks uint64     `json:"sequence_breaks,omitempty"`
}

type walDelta struct {
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
	IntervalBucket      string      `json:"interval_bucket,omitempty"`
	DurationBucket      string      `json:"duration_bucket,omitempty"`
	CountBucket         string      `json:"count_bucket,omitempty"`
	Reason              string      `json:"reason,omitempty"`
	Flags               string      `json:"flags,omitempty"`
	Day                 string      `json:"day"`
	Attempts            uint64      `json:"attempts"`
	Successes           uint64      `json:"successes"`
	Failures            uint64      `json:"failures"`
}

var errWALNeedsCompaction = errors.New("usage WAL requires compaction")

type walAppendIO struct {
	write func(*os.File, []byte) (int, error)
	sync  func(*os.File) error
}

var defaultWALAppendIO = walAppendIO{
	write: func(file *os.File, data []byte) (int, error) { return file.Write(data) },
	sync:  func(file *os.File) error { return file.Sync() },
}

func prepareDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create usage directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect usage directory: %w", err)
	}
	return nil
}

func ensureWAL(directory string) error {
	file, err := os.OpenFile(WALPath(directory), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open usage WAL: %w", err)
	}
	closeErr := file.Close()
	if err := os.Chmod(WALPath(directory), 0o600); err != nil {
		return fmt.Errorf("protect usage WAL: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close usage WAL: %w", closeErr)
	}
	return nil
}

func emptyAggregate() aggregateState {
	return aggregateState{Entries: make(map[entryKey]*aggregateEntry)}
}

func loadAggregate(directory string) (aggregateState, error, error) {
	mainPath := SnapshotPath(directory)
	backupPath := snapshotBackupPath(directory)

	state, mainErr := readSnapshot(mainPath)
	if mainErr == nil {
		return state, nil, nil
	}
	mainMissing := errors.Is(mainErr, os.ErrNotExist)

	backup, backupErr := readSnapshot(backupPath)
	if backupErr == nil {
		if !mainMissing {
			if err := os.Remove(mainPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return aggregateState{}, nil, fmt.Errorf("remove corrupt usage snapshot: %w", err)
			}
		}
		return backup, fmt.Errorf("recover usage snapshot from backup: %w", mainErr), nil
	}
	if mainMissing && errors.Is(backupErr, os.ErrNotExist) {
		return emptyAggregate(), nil, nil
	}
	if mainMissing {
		return aggregateState{}, nil, fmt.Errorf("load usage snapshot backup: %w", backupErr)
	}
	if errors.Is(backupErr, os.ErrNotExist) {
		return aggregateState{}, nil, fmt.Errorf("load usage snapshot: %w", mainErr)
	}
	return aggregateState{}, nil, fmt.Errorf("load usage snapshot: %v; backup: %w", mainErr, backupErr)
}

func readSnapshot(path string) (aggregateState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return aggregateState{}, err
	}
	return decodeSnapshot(data, path)
}

func decodeSnapshot(data []byte, path string) (aggregateState, error) {
	if len(data) > maximumSnapshotBytes {
		return aggregateState{}, fmt.Errorf("decode %s: snapshot exceeds %d bytes", filepath.Base(path), maximumSnapshotBytes)
	}
	var persisted snapshot
	if err := json.Unmarshal(data, &persisted); err != nil {
		return aggregateState{}, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	if !supportedSchemaVersion(persisted.SchemaVersion) {
		return aggregateState{}, fmt.Errorf("decode %s: unsupported schema_version %d", filepath.Base(path), persisted.SchemaVersion)
	}
	if persisted.UpdatedDay != "" {
		if err := validateDay(persisted.UpdatedDay); err != nil {
			return aggregateState{}, fmt.Errorf("decode %s: updated_day: %w", filepath.Base(path), err)
		}
	}
	state := aggregateState{
		UpdatedDay:     persisted.UpdatedDay,
		AppliedThrough: persisted.AppliedThrough,
		Inventory:      cloneInventory(persisted.Inventory),
		Strategies:     cloneStrategies(persisted.Strategies),
		Controls:       cloneStrings(persisted.Controls),
		Entries:        make(map[entryKey]*aggregateEntry, len(persisted.Entries)),
		Dropped:        persisted.Dropped,
		InvalidDropped: persisted.InvalidDropped,
		Coalesced:      persisted.Coalesced,
		SequenceBreaks: persisted.SequenceBreaks,
	}
	for index, item := range persisted.Inventory {
		if !validResolution(item.Resolution) {
			return aggregateState{}, fmt.Errorf("decode %s: inventory[%d] has invalid resolution %q", filepath.Base(path), index, item.Resolution)
		}
	}
	for index, strategy := range persisted.Strategies {
		if strategy.ID == "" {
			return aggregateState{}, fmt.Errorf("decode %s: strategies[%d] has empty id", filepath.Base(path), index)
		}
		for definitionIndex, definition := range strategy.Inventory {
			if !validResolution(definition.Resolution) {
				return aggregateState{}, fmt.Errorf("decode %s: strategies[%d].inventory[%d] has invalid resolution %q", filepath.Base(path), index, definitionIndex, definition.Resolution)
			}
		}
	}
	for index, item := range persisted.Entries {
		if item.Kind != EventLegacy && !validEventKind(item.Kind) {
			return aggregateState{}, fmt.Errorf("decode %s: entries[%d] has invalid kind %q", filepath.Base(path), index, item.Kind)
		}
		if !validResolution(item.Resolution) {
			return aggregateState{}, fmt.Errorf("decode %s: entries[%d] has invalid resolution %q", filepath.Base(path), index, item.Resolution)
		}
		if item.CandidateResolution != "" && !validResolution(item.CandidateResolution) {
			return aggregateState{}, fmt.Errorf("decode %s: entries[%d] has invalid candidate resolution %q", filepath.Base(path), index, item.CandidateResolution)
		}
		if item.GestureKind != "" && !validGestureKind(item.GestureKind) {
			return aggregateState{}, fmt.Errorf("decode %s: entries[%d] has invalid gesture kind %q", filepath.Base(path), index, item.GestureKind)
		}
		if item.Successes > item.Attempts || item.Failures > item.Attempts-item.Successes {
			return aggregateState{}, fmt.Errorf("decode %s: entries[%d] has inconsistent counters", filepath.Base(path), index)
		}
		if item.FirstUsedDay != "" {
			if err := validateDay(item.FirstUsedDay); err != nil {
				return aggregateState{}, fmt.Errorf("decode %s: entries[%d] first_used_day: %w", filepath.Base(path), index, err)
			}
		}
		if item.LastUsedDay != "" {
			if err := validateDay(item.LastUsedDay); err != nil {
				return aggregateState{}, fmt.Errorf("decode %s: entries[%d] last_used_day: %w", filepath.Base(path), index, err)
			}
		}
		daily := make(map[string]counters, len(item.Daily))
		for day, value := range item.Daily {
			if err := validateDay(day); err != nil {
				return aggregateState{}, fmt.Errorf("decode %s: entries[%d] daily: %w", filepath.Base(path), index, err)
			}
			if value.Successes > value.Attempts || value.Failures > value.Attempts-value.Successes {
				return aggregateState{}, fmt.Errorf("decode %s: entries[%d] daily %s has inconsistent counters", filepath.Base(path), index, day)
			}
			daily[day] = value
		}
		key := entryKey{
			Kind:                item.Kind,
			StrategyID:          item.StrategyID,
			ForegroundApp:       item.ForegroundApp,
			ActiveProfile:       item.ActiveProfile,
			BindingProfile:      item.BindingProfile,
			Control:             item.Control,
			Gesture:             item.Gesture,
			PhysicalGesture:     item.PhysicalGesture,
			GestureKind:         item.GestureKind,
			Action:              item.Action,
			RelatedGesture:      item.RelatedGesture,
			RelatedAction:       item.RelatedAction,
			Resolution:          item.Resolution,
			CandidateResolution: item.CandidateResolution,
			IntervalBucket:      item.IntervalBucket,
			DurationBucket:      item.DurationBucket,
			CountBucket:         item.CountBucket,
			Reason:              item.Reason,
			Flags:               item.Flags,
		}
		if _, exists := state.Entries[key]; exists {
			return aggregateState{}, fmt.Errorf("decode %s: duplicate aggregate entry at index %d", filepath.Base(path), index)
		}
		state.Entries[key] = &aggregateEntry{
			Key:          key,
			Attempts:     item.Attempts,
			Successes:    item.Successes,
			Failures:     item.Failures,
			FirstUsedDay: item.FirstUsedDay,
			LastUsedDay:  item.LastUsedDay,
			Daily:        daily,
		}
	}
	return state, nil
}

func replayWAL(directory string, state *aggregateState) error {
	path := WALPath(directory)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read usage WAL: %w", err)
	}

	completeLength := 0
	if index := bytes.LastIndexByte(data, '\n'); index >= 0 {
		completeLength = index + 1
	}
	for _, line := range bytes.Split(data[:completeLength], []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var batch walBatch
		if err := json.Unmarshal(line, &batch); err != nil {
			return fmt.Errorf("decode usage WAL: %w", err)
		}
		if err := validateWALBatch(batch); err != nil {
			return err
		}
		if batch.BatchID <= state.AppliedThrough {
			continue
		}
		if batch.BatchID != state.AppliedThrough+1 {
			return fmt.Errorf("decode usage WAL: expected batch %d, got %d", state.AppliedThrough+1, batch.BatchID)
		}
		applyWALBatch(state, batch)
	}
	if completeLength != len(data) {
		if err := truncateWAL(path, int64(completeLength)); err != nil {
			return err
		}
	}
	return nil
}

func validateWALBatch(batch walBatch) error {
	if !supportedSchemaVersion(batch.SchemaVersion) {
		return fmt.Errorf("decode usage WAL: unsupported schema_version %d", batch.SchemaVersion)
	}
	if batch.BatchID == 0 {
		return errors.New("decode usage WAL: batch_id must be positive")
	}
	for index, delta := range batch.Deltas {
		if delta.Kind != EventLegacy && !validEventKind(delta.Kind) {
			return fmt.Errorf("decode usage WAL: deltas[%d] has invalid kind %q", index, delta.Kind)
		}
		if !validResolution(delta.Resolution) {
			return fmt.Errorf("decode usage WAL: deltas[%d] has invalid resolution %q", index, delta.Resolution)
		}
		if delta.CandidateResolution != "" && !validResolution(delta.CandidateResolution) {
			return fmt.Errorf("decode usage WAL: deltas[%d] has invalid candidate resolution %q", index, delta.CandidateResolution)
		}
		if delta.GestureKind != "" && !validGestureKind(delta.GestureKind) {
			return fmt.Errorf("decode usage WAL: deltas[%d] has invalid gesture kind %q", index, delta.GestureKind)
		}
		if err := validateDay(delta.Day); err != nil {
			return fmt.Errorf("decode usage WAL: deltas[%d] day: %w", index, err)
		}
		if delta.Attempts == 0 || delta.Successes > delta.Attempts || delta.Failures > delta.Attempts-delta.Successes {
			return fmt.Errorf("decode usage WAL: deltas[%d] has inconsistent counters", index)
		}
	}
	return nil
}

func supportedSchemaVersion(value int) bool {
	return value == legacySchemaVersion || value == schemaVersion
}

func truncateWAL(path string, size int64) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open usage WAL for recovery: %w", err)
	}
	defer file.Close()
	if err := file.Truncate(size); err != nil {
		return fmt.Errorf("truncate partial usage WAL: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync recovered usage WAL: %w", err)
	}
	return nil
}

func makeWALBatch(batchID uint64, pending map[deltaKey]counters, dropped uint64) walBatch {
	deltas := make([]walDelta, 0, len(pending))
	for key, value := range pending {
		deltas = append(deltas, walDelta{
			Kind:                key.Entry.Kind,
			StrategyID:          key.Entry.StrategyID,
			ForegroundApp:       key.Entry.ForegroundApp,
			ActiveProfile:       key.Entry.ActiveProfile,
			BindingProfile:      key.Entry.BindingProfile,
			Control:             key.Entry.Control,
			Gesture:             key.Entry.Gesture,
			PhysicalGesture:     key.Entry.PhysicalGesture,
			GestureKind:         key.Entry.GestureKind,
			Action:              key.Entry.Action,
			RelatedGesture:      key.Entry.RelatedGesture,
			RelatedAction:       key.Entry.RelatedAction,
			Resolution:          key.Entry.Resolution,
			CandidateResolution: key.Entry.CandidateResolution,
			IntervalBucket:      key.Entry.IntervalBucket,
			DurationBucket:      key.Entry.DurationBucket,
			CountBucket:         key.Entry.CountBucket,
			Reason:              key.Entry.Reason,
			Flags:               key.Entry.Flags,
			Day:                 key.Day,
			Attempts:            value.Attempts,
			Successes:           value.Successes,
			Failures:            value.Failures,
		})
	}
	sort.Slice(deltas, func(left, right int) bool {
		return lessDelta(deltas[left], deltas[right])
	})
	return walBatch{SchemaVersion: schemaVersion, BatchID: batchID, Deltas: deltas, Dropped: dropped}
}

func appendWALBatch(directory string, batch walBatch) error {
	return appendWALBatchWithIO(directory, batch, defaultWALAppendIO)
}

func appendWALBatchWithIO(directory string, batch walBatch, operations walAppendIO) error {
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("encode usage WAL batch: %w", err)
	}
	data = append(data, '\n')
	path := WALPath(directory)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open usage WAL: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect usage WAL: %w", err)
	}
	originalSize, seekErr := file.Seek(0, io.SeekEnd)
	if seekErr != nil {
		_ = file.Close()
		return fmt.Errorf("seek usage WAL append position: %w", seekErr)
	}
	if originalSize+int64(len(data)) > walMaximumSize {
		_ = file.Close()
		return fmt.Errorf("%w: maximum is %d bytes", errWALNeedsCompaction, walMaximumSize)
	}

	written, writeErr := operations.write(file, data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = operations.sync(file)
	}
	if writeErr != nil {
		rollbackErr := rollbackWALAppend(file, originalSize, operations.sync)
		closeErr := file.Close()
		if rollbackErr != nil || closeErr != nil {
			return fmt.Errorf("append usage WAL: %w", errors.Join(writeErr, rollbackErr, closeErr))
		}
		return fmt.Errorf("append usage WAL: %w", writeErr)
	}
	closeErr := file.Close()
	if closeErr != nil {
		return fmt.Errorf("close usage WAL: %w", closeErr)
	}
	return nil
}

func rollbackWALAppend(file *os.File, originalSize int64, syncFile func(*os.File) error) error {
	truncateErr := file.Truncate(originalSize)
	if truncateErr != nil {
		return fmt.Errorf("rollback usage WAL length: %w", truncateErr)
	}
	if syncErr := syncFile(file); syncErr != nil {
		return fmt.Errorf("sync rolled back usage WAL: %w", syncErr)
	}
	return nil
}

func applyWALBatch(state *aggregateState, batch walBatch) {
	if state.Entries == nil {
		state.Entries = make(map[entryKey]*aggregateEntry)
	}
	for _, delta := range batch.Deltas {
		key := entryKey{
			Kind:                delta.Kind,
			StrategyID:          delta.StrategyID,
			ForegroundApp:       delta.ForegroundApp,
			ActiveProfile:       delta.ActiveProfile,
			BindingProfile:      delta.BindingProfile,
			Control:             delta.Control,
			Gesture:             delta.Gesture,
			PhysicalGesture:     delta.PhysicalGesture,
			GestureKind:         delta.GestureKind,
			Action:              delta.Action,
			RelatedGesture:      delta.RelatedGesture,
			RelatedAction:       delta.RelatedAction,
			Resolution:          delta.Resolution,
			CandidateResolution: delta.CandidateResolution,
			IntervalBucket:      delta.IntervalBucket,
			DurationBucket:      delta.DurationBucket,
			CountBucket:         delta.CountBucket,
			Reason:              delta.Reason,
			Flags:               delta.Flags,
		}
		entry := state.Entries[key]
		if entry == nil {
			entry = &aggregateEntry{Key: key, Daily: make(map[string]counters)}
			state.Entries[key] = entry
		}
		lifetime := addCounters(counters{
			Attempts:  entry.Attempts,
			Successes: entry.Successes,
			Failures:  entry.Failures,
		}, counters{
			Attempts:  delta.Attempts,
			Successes: delta.Successes,
			Failures:  delta.Failures,
		})
		entry.Attempts = lifetime.Attempts
		entry.Successes = lifetime.Successes
		entry.Failures = lifetime.Failures
		if entry.FirstUsedDay == "" || delta.Day < entry.FirstUsedDay {
			entry.FirstUsedDay = delta.Day
		}
		if entry.LastUsedDay == "" || delta.Day > entry.LastUsedDay {
			entry.LastUsedDay = delta.Day
		}
		entry.Daily[delta.Day] = addCounters(entry.Daily[delta.Day], counters{
			Attempts:  delta.Attempts,
			Successes: delta.Successes,
			Failures:  delta.Failures,
		})
	}
	state.Dropped = saturatingAdd(state.Dropped, batch.Dropped)
	state.InvalidDropped = saturatingAdd(state.InvalidDropped, batch.InvalidDropped)
	state.Coalesced = saturatingAdd(state.Coalesced, batch.Coalesced)
	state.SequenceBreaks = saturatingAdd(state.SequenceBreaks, batch.SequenceBreaks)
	state.AppliedThrough = batch.BatchID
}

func maybeCompact(directory string, state *aggregateState, now time.Time, refreshDue bool) (bool, error) {
	info, err := os.Stat(WALPath(directory))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect usage WAL: %w", err)
	}
	if info.Size() == 0 || (!refreshDue && info.Size() < walCompactSize) {
		return false, nil
	}
	if err := compact(directory, state, now); err != nil {
		return false, err
	}
	return true, nil
}

func compact(directory string, state *aggregateState, now time.Time) error {
	pruneDaily(state, now)
	enforceAggregateLimits(state)
	state.UpdatedDay = dayString(now)
	data, err := enforceSnapshotByteLimit(state, maximumSnapshotBytes)
	if err != nil {
		return err
	}
	if err := replaceSnapshot(directory, data); err != nil {
		return err
	}
	if err := resetWAL(directory); err != nil {
		return err
	}
	return nil
}

func encodeSnapshot(state aggregateState) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(makeSnapshot(state)); err != nil {
		return nil, fmt.Errorf("encode usage snapshot: %w", err)
	}
	return output.Bytes(), nil
}

func enforceSnapshotByteLimit(state *aggregateState, maximum int) ([]byte, error) {
	if maximum <= 0 {
		return nil, errors.New("encode usage snapshot: maximum size must be positive")
	}
	for {
		data, err := encodeSnapshot(*state)
		if err != nil {
			return nil, err
		}
		if len(data) <= maximum {
			return data, nil
		}
		detailed := detailedEntryCount(state.Entries)
		if detailed == 0 {
			return nil, fmt.Errorf("encode usage snapshot: minimum aggregate is %d bytes; maximum is %d", len(data), maximum)
		}
		keepCount := detailed * maximum / len(data)
		keepCount = keepCount * 9 / 10
		if keepCount >= detailed {
			keepCount = detailed - 1
		}
		coalesceDetailedEntryTail(state, keepCount)
	}
}

func detailedEntryCount(entries map[entryKey]*aggregateEntry) int {
	total := 0
	for key := range entries {
		if key != overflowEntryKey(key) {
			total++
		}
	}
	return total
}

func coalesceDetailedEntryTail(state *aggregateState, keepCount int) {
	var detailed []*aggregateEntry
	limited := make(map[entryKey]*aggregateEntry, keepCount+2)
	for _, entry := range state.Entries {
		if entry.Key == overflowEntryKey(entry.Key) {
			mergeAggregateEntry(limited, entry.Key, entry)
			continue
		}
		detailed = append(detailed, entry)
	}
	sort.Slice(detailed, func(left, right int) bool {
		if detailed[left].Attempts != detailed[right].Attempts {
			return detailed[left].Attempts > detailed[right].Attempts
		}
		return entryKeySortValue(detailed[left].Key) < entryKeySortValue(detailed[right].Key)
	})
	if keepCount < 0 {
		keepCount = 0
	}
	if keepCount > len(detailed) {
		keepCount = len(detailed)
	}
	for index, entry := range detailed {
		key := entry.Key
		if index >= keepCount {
			key = overflowEntryKey(key)
			state.Coalesced = saturatingAdd(state.Coalesced, entry.Attempts)
		}
		mergeAggregateEntry(limited, key, entry)
	}
	state.Entries = limited
}

func makeSnapshot(state aggregateState) snapshot {
	entries := make([]snapshotEntry, 0, len(state.Entries))
	for _, entry := range state.Entries {
		daily := make(map[string]counters, len(entry.Daily))
		for day, value := range entry.Daily {
			daily[day] = value
		}
		entries = append(entries, snapshotEntry{
			Kind:                entry.Key.Kind,
			StrategyID:          entry.Key.StrategyID,
			ForegroundApp:       entry.Key.ForegroundApp,
			ActiveProfile:       entry.Key.ActiveProfile,
			BindingProfile:      entry.Key.BindingProfile,
			Control:             entry.Key.Control,
			Gesture:             entry.Key.Gesture,
			PhysicalGesture:     entry.Key.PhysicalGesture,
			GestureKind:         entry.Key.GestureKind,
			Action:              entry.Key.Action,
			RelatedGesture:      entry.Key.RelatedGesture,
			RelatedAction:       entry.Key.RelatedAction,
			Resolution:          entry.Key.Resolution,
			CandidateResolution: entry.Key.CandidateResolution,
			IntervalBucket:      entry.Key.IntervalBucket,
			DurationBucket:      entry.Key.DurationBucket,
			CountBucket:         entry.Key.CountBucket,
			Reason:              entry.Key.Reason,
			Flags:               entry.Key.Flags,
			Attempts:            entry.Attempts,
			Successes:           entry.Successes,
			Failures:            entry.Failures,
			FirstUsedDay:        entry.FirstUsedDay,
			LastUsedDay:         entry.LastUsedDay,
			Daily:               daily,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return lessSnapshotEntry(entries[left], entries[right])
	})
	return snapshot{
		SchemaVersion:  schemaVersion,
		UpdatedDay:     state.UpdatedDay,
		AppliedThrough: state.AppliedThrough,
		Inventory:      cloneInventory(state.Inventory),
		Strategies:     cloneStrategies(state.Strategies),
		Controls:       cloneStrings(state.Controls),
		Entries:        entries,
		Dropped:        state.Dropped,
		InvalidDropped: state.InvalidDropped,
		Coalesced:      state.Coalesced,
		SequenceBreaks: state.SequenceBreaks,
	}
}

func replaceSnapshot(directory string, data []byte) error {
	if len(data) > maximumSnapshotBytes {
		return fmt.Errorf("replace usage snapshot: data exceeds %d bytes", maximumSnapshotBytes)
	}
	path := SnapshotPath(directory)
	temporary := path + ".tmp"
	backup := snapshotBackupPath(directory)

	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create usage snapshot temporary file: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("protect usage snapshot temporary file: %w", err)
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("write usage snapshot temporary file: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("close usage snapshot temporary file: %w", closeErr)
	}

	hadCurrent := false
	if _, err := os.Stat(path); err == nil {
		hadCurrent = true
		if err := removeForReplace(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(temporary)
			return fmt.Errorf("remove old usage snapshot backup: %w", err)
		}
		if err := renameForReplace(path, backup); err != nil {
			_ = os.Remove(temporary)
			return fmt.Errorf("backup usage snapshot: %w", err)
		}
	}
	if err := renameForReplace(temporary, path); err != nil {
		if hadCurrent {
			_ = renameForReplace(backup, path)
		}
		_ = os.Remove(temporary)
		return fmt.Errorf("replace usage snapshot: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect usage snapshot: %w", err)
	}
	// The WAL is reset immediately after this function returns. Mirror the new
	// snapshot into the recovery copy first so both files are in the same batch
	// generation if the primary snapshot is later damaged.
	if err := replaceSnapshotBackup(backup, data); err != nil {
		return err
	}
	return nil
}

func replaceSnapshotBackup(backup string, data []byte) error {
	temporary := backup + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create usage snapshot backup temporary file: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("protect usage snapshot backup temporary file: %w", err)
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("write usage snapshot backup temporary file: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("close usage snapshot backup temporary file: %w", closeErr)
	}
	if err := removeForReplace(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(temporary)
		return fmt.Errorf("remove old usage snapshot backup mirror: %w", err)
	}
	if err := renameForReplace(temporary, backup); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace usage snapshot backup mirror: %w", err)
	}
	if err := os.Chmod(backup, 0o600); err != nil {
		return fmt.Errorf("protect usage snapshot backup mirror: %w", err)
	}
	return nil
}

func resetWAL(directory string) error {
	path := WALPath(directory)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("reset usage WAL: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect usage WAL: %w", err)
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return fmt.Errorf("sync reset usage WAL: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close reset usage WAL: %w", closeErr)
	}
	return nil
}

func pruneDaily(state *aggregateState, now time.Time) {
	current := now
	today := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
	cutoff := dayString(today.AddDate(0, 0, -(recentDayCount - 1)))
	for key, entry := range state.Entries {
		for day := range entry.Daily {
			if day < cutoff {
				delete(entry.Daily, day)
			}
		}
		entry.Attempts = 0
		entry.Successes = 0
		entry.Failures = 0
		entry.FirstUsedDay = ""
		entry.LastUsedDay = ""
		for day, value := range entry.Daily {
			lifetime := addCounters(counters{
				Attempts: entry.Attempts, Successes: entry.Successes, Failures: entry.Failures,
			}, value)
			entry.Attempts = lifetime.Attempts
			entry.Successes = lifetime.Successes
			entry.Failures = lifetime.Failures
			if entry.FirstUsedDay == "" || day < entry.FirstUsedDay {
				entry.FirstUsedDay = day
			}
			if day > entry.LastUsedDay {
				entry.LastUsedDay = day
			}
		}
		if entry.Attempts == 0 {
			delete(state.Entries, key)
		}
	}
}

func enforceAggregateLimits(state *aggregateState) {
	if state.Entries == nil {
		state.Entries = make(map[entryKey]*aggregateEntry)
	}
	type appUse struct {
		name  string
		count uint64
	}
	apps := make(map[string]appUse)
	for _, entry := range state.Entries {
		name, err := sanitizeAppName(entry.Key.ForegroundApp)
		if err != nil {
			name = otherAppBucket
		}
		if name == "" || name == otherAppBucket {
			continue
		}
		canonical := strings.ToLower(name)
		value := apps[canonical]
		if value.name == "" {
			value.name = name
		}
		value.count = saturatingAdd(value.count, entry.Attempts)
		apps[canonical] = value
	}
	orderedApps := make([]appUse, 0, len(apps))
	for _, value := range apps {
		orderedApps = append(orderedApps, value)
	}
	sort.Slice(orderedApps, func(left, right int) bool {
		if orderedApps[left].count != orderedApps[right].count {
			return orderedApps[left].count > orderedApps[right].count
		}
		return strings.ToLower(orderedApps[left].name) < strings.ToLower(orderedApps[right].name)
	})
	if len(orderedApps) > maximumAppBuckets {
		orderedApps = orderedApps[:maximumAppBuckets]
	}
	keptApps := make(map[string]string, len(orderedApps))
	for _, value := range orderedApps {
		keptApps[strings.ToLower(value.name)] = value.name
	}

	normalized := make(map[entryKey]*aggregateEntry, len(state.Entries))
	for _, entry := range state.Entries {
		key := entry.Key
		name, err := sanitizeAppName(key.ForegroundApp)
		if err != nil {
			name = otherAppBucket
			state.Coalesced = saturatingAdd(state.Coalesced, entry.Attempts)
		}
		if name != "" && name != otherAppBucket {
			if kept, found := keptApps[strings.ToLower(name)]; found {
				name = kept
			} else {
				name = otherAppBucket
				state.Coalesced = saturatingAdd(state.Coalesced, entry.Attempts)
			}
		}
		key.ForegroundApp = name
		mergeAggregateEntry(normalized, key, entry)
	}
	state.Entries = normalized

	if len(state.Entries) > maximumEntries {
		entries := make([]*aggregateEntry, 0, len(state.Entries))
		for _, entry := range state.Entries {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(left, right int) bool {
			if entries[left].Attempts != entries[right].Attempts {
				return entries[left].Attempts > entries[right].Attempts
			}
			return entryKeySortValue(entries[left].Key) < entryKeySortValue(entries[right].Key)
		})
		const overflowReserve = 4
		keepCount := maximumEntries - overflowReserve
		limited := make(map[entryKey]*aggregateEntry, maximumEntries)
		for index, entry := range entries {
			key := entry.Key
			if index >= keepCount {
				key = overflowEntryKey(key)
				state.Coalesced = saturatingAdd(state.Coalesced, entry.Attempts)
			}
			mergeAggregateEntry(limited, key, entry)
		}
		state.Entries = limited
	}
	enforceDailyCellLimit(state)
	if len(state.Strategies) > maximumStrategies {
		state.Strategies = append([]StrategyDefinition(nil), state.Strategies[len(state.Strategies)-maximumStrategies:]...)
		state.Coalesced = saturatingAdd(state.Coalesced, 1)
	}
	for index := range state.Strategies {
		if len(state.Strategies[index].Inventory) > maximumInventoryDefinitions {
			excess := len(state.Strategies[index].Inventory) - maximumInventoryDefinitions
			state.Strategies[index].Inventory = cloneInventory(state.Strategies[index].Inventory[:maximumInventoryDefinitions])
			state.Coalesced = saturatingAdd(state.Coalesced, uint64(excess))
		}
	}
}

// enforceDailyCellLimit bounds the sparse entry/day matrix. Entries are kept
// in descending frequency order; the lower-frequency tail is merged into the
// same primary/diagnostic overflow keys used by the cardinality limit. Within
// the normal 90-day retention window, those overflow keys need at most 180
// cells, so no retained counts are discarded.
func enforceDailyCellLimit(state *aggregateState) {
	if dailyCellCount(state.Entries) <= maximumDailyCells {
		return
	}
	entries := make([]*aggregateEntry, 0, len(state.Entries))
	for _, entry := range state.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Attempts != entries[right].Attempts {
			return entries[left].Attempts > entries[right].Attempts
		}
		return entryKeySortValue(entries[left].Key) < entryKeySortValue(entries[right].Key)
	})

	// Track how many tail entries contribute each day to each overflow key.
	// Moving an entry into the kept prefix removes a cell from the overflow only
	// when it was the last tail contributor for that day.
	remaining := make(map[entryKey]map[string]int, 2)
	for _, entry := range entries {
		key := overflowEntryKey(entry.Key)
		dayCounts := remaining[key]
		if dayCounts == nil {
			dayCounts = make(map[string]int)
			remaining[key] = dayCounts
		}
		for day := range entry.Daily {
			dayCounts[day]++
		}
	}
	overflowCells := 0
	for _, dayCounts := range remaining {
		overflowCells += len(dayCounts)
	}
	keptCells := 0
	keepCount := 0
	if overflowCells <= maximumDailyCells {
		for _, entry := range entries {
			dayCounts := remaining[overflowEntryKey(entry.Key)]
			removedOverflowCells := 0
			for day := range entry.Daily {
				if dayCounts[day] == 1 {
					removedOverflowCells++
				}
			}
			candidateCells := keptCells + len(entry.Daily) + overflowCells - removedOverflowCells
			if candidateCells > maximumDailyCells {
				break
			}
			keptCells += len(entry.Daily)
			overflowCells -= removedOverflowCells
			for day := range entry.Daily {
				dayCounts[day]--
				if dayCounts[day] == 0 {
					delete(dayCounts, day)
				}
			}
			keepCount++
		}
	}

	limited := make(map[entryKey]*aggregateEntry, keepCount+len(remaining))
	for index, entry := range entries {
		key := entry.Key
		if index >= keepCount {
			key = overflowEntryKey(key)
			if key != entry.Key {
				state.Coalesced = saturatingAdd(state.Coalesced, entry.Attempts)
			}
		}
		mergeAggregateEntry(limited, key, entry)
	}
	state.Entries = limited

	// A valid compacted aggregate has at most 90 days per overflow key. This
	// fallback also protects direct or adversarial callers that skipped pruning:
	// old day cells are folded into the oldest retained day without losing
	// counters, keeping the hard matrix bound intact.
	if dailyCellCount(state.Entries) > maximumDailyCells {
		coalesceOverflowDays(state)
	}
}

func dailyCellCount(entries map[entryKey]*aggregateEntry) int {
	total := 0
	for _, entry := range entries {
		total += len(entry.Daily)
	}
	return total
}

func coalesceOverflowDays(state *aggregateState) {
	entries := make([]*aggregateEntry, 0, len(state.Entries))
	for _, entry := range state.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Attempts != entries[right].Attempts {
			return entries[left].Attempts > entries[right].Attempts
		}
		return entryKeySortValue(entries[left].Key) < entryKeySortValue(entries[right].Key)
	})
	remainingCells := maximumDailyCells
	for index, entry := range entries {
		remainingEntries := len(entries) - index - 1
		allowance := remainingCells - remainingEntries
		if allowance < 1 {
			allowance = 1
		}
		if len(entry.Daily) > allowance {
			state.Coalesced = saturatingAdd(state.Coalesced, coalesceEntryDays(entry, allowance))
		}
		remainingCells -= len(entry.Daily)
	}
}

func coalesceEntryDays(entry *aggregateEntry, maximum int) uint64 {
	if len(entry.Daily) <= maximum {
		return 0
	}
	days := make([]string, 0, len(entry.Daily))
	for day := range entry.Daily {
		days = append(days, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	retained := days[:maximum]
	foldInto := retained[len(retained)-1]
	folded := counters{}
	var coalesced uint64
	for _, day := range days[maximum:] {
		value := entry.Daily[day]
		folded = addCounters(folded, value)
		coalesced = saturatingAdd(coalesced, value.Attempts)
		delete(entry.Daily, day)
	}
	entry.Daily[foldInto] = addCounters(entry.Daily[foldInto], folded)
	entry.FirstUsedDay = retained[len(retained)-1]
	entry.LastUsedDay = retained[0]
	return coalesced
}

func mergeAggregateEntry(destination map[entryKey]*aggregateEntry, key entryKey, source *aggregateEntry) {
	entry := destination[key]
	if entry == nil {
		entry = &aggregateEntry{Key: key, Daily: make(map[string]counters)}
		destination[key] = entry
	}
	for day, value := range source.Daily {
		entry.Daily[day] = addCounters(entry.Daily[day], value)
	}
	entry.Attempts = 0
	entry.Successes = 0
	entry.Failures = 0
	entry.FirstUsedDay = ""
	entry.LastUsedDay = ""
	for day, value := range entry.Daily {
		total := addCounters(counters{
			Attempts: entry.Attempts, Successes: entry.Successes, Failures: entry.Failures,
		}, value)
		entry.Attempts = total.Attempts
		entry.Successes = total.Successes
		entry.Failures = total.Failures
		if entry.FirstUsedDay == "" || day < entry.FirstUsedDay {
			entry.FirstUsedDay = day
		}
		if day > entry.LastUsedDay {
			entry.LastUsedDay = day
		}
	}
}

func entryKeySortValue(key entryKey) string {
	return strings.Join(entryKeyValues(key), "\x00")
}

func snapshotBackupPath(directory string) string {
	return SnapshotPath(directory) + ".bak"
}

func validateDay(value string) error {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || dayString(parsed) != value {
		if err == nil {
			err = errors.New("non-canonical date")
		}
		return fmt.Errorf("invalid local day %q: %w", value, err)
	}
	return nil
}

func lessDelta(left, right walDelta) bool {
	leftValues := append(entryKeyValues(entryKeyFromDelta(left)), left.Day)
	rightValues := append(entryKeyValues(entryKeyFromDelta(right)), right.Day)
	for index := range leftValues {
		if leftValues[index] != rightValues[index] {
			return leftValues[index] < rightValues[index]
		}
	}
	return false
}

func lessSnapshotEntry(left, right snapshotEntry) bool {
	leftValues := entryKeyValues(entryKeyFromSnapshotEntry(left))
	rightValues := entryKeyValues(entryKeyFromSnapshotEntry(right))
	for index := range leftValues {
		if leftValues[index] != rightValues[index] {
			return leftValues[index] < rightValues[index]
		}
	}
	return false
}

func entryKeyFromDelta(value walDelta) entryKey {
	return entryKey{
		Kind: value.Kind, StrategyID: value.StrategyID, ForegroundApp: value.ForegroundApp,
		ActiveProfile: value.ActiveProfile, BindingProfile: value.BindingProfile,
		Control: value.Control, Gesture: value.Gesture, PhysicalGesture: value.PhysicalGesture,
		GestureKind: value.GestureKind, Action: value.Action, RelatedGesture: value.RelatedGesture,
		RelatedAction: value.RelatedAction, Resolution: value.Resolution,
		CandidateResolution: value.CandidateResolution, IntervalBucket: value.IntervalBucket,
		DurationBucket: value.DurationBucket, CountBucket: value.CountBucket,
		Reason: value.Reason, Flags: value.Flags,
	}
}

func entryKeyFromSnapshotEntry(value snapshotEntry) entryKey {
	return entryKey{
		Kind: value.Kind, StrategyID: value.StrategyID, ForegroundApp: value.ForegroundApp,
		ActiveProfile: value.ActiveProfile, BindingProfile: value.BindingProfile,
		Control: value.Control, Gesture: value.Gesture, PhysicalGesture: value.PhysicalGesture,
		GestureKind: value.GestureKind, Action: value.Action, RelatedGesture: value.RelatedGesture,
		RelatedAction: value.RelatedAction, Resolution: value.Resolution,
		CandidateResolution: value.CandidateResolution, IntervalBucket: value.IntervalBucket,
		DurationBucket: value.DurationBucket, CountBucket: value.CountBucket,
		Reason: value.Reason, Flags: value.Flags,
	}
}

func entryKeyValues(value entryKey) []string {
	return []string{
		string(value.Kind), value.StrategyID, value.ForegroundApp, value.ActiveProfile,
		value.BindingProfile, value.Control, value.Gesture, value.PhysicalGesture,
		string(value.GestureKind), value.Action, value.RelatedGesture, value.RelatedAction,
		string(value.Resolution), string(value.CandidateResolution), value.IntervalBucket,
		value.DurationBucket, value.CountBucket, value.Reason, value.Flags,
	}
}
