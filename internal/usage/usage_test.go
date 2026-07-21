package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func TestOpenImmediatelyPersistsInventory(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 7, 20, 0, 30, 0, 0, testLocation)
	inventory := []BindingDefinition{
		{Profile: "default", Gesture: "a", Action: "click_left", Resolution: ResolutionBound},
		{Profile: "default", Gesture: "r3", Resolution: ResolutionDisabled},
	}
	recorder, err := Open(Options{
		Directory: directory,
		Inventory: inventory,
		Controls:  []string{"a", "r3"},
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()

	persisted := readPersistedSnapshot(t, directory)
	if persisted.SchemaVersion != schemaVersion {
		t.Fatalf("schema_version = %d, want %d", persisted.SchemaVersion, schemaVersion)
	}
	if persisted.UpdatedDay != "2026-07-20" {
		t.Fatalf("updated_day = %q, want local day 2026-07-20", persisted.UpdatedDay)
	}
	if len(persisted.Inventory) != len(inventory) || persisted.Inventory[1] != inventory[1] {
		t.Fatalf("inventory = %#v, want %#v", persisted.Inventory, inventory)
	}
	if strings.Join(persisted.Controls, ",") != "a,r3" {
		t.Fatalf("controls = %#v", persisted.Controls)
	}
	if len(persisted.Entries) != 0 {
		t.Fatalf("entries = %#v, want empty", persisted.Entries)
	}
	assertPrivateFile(t, SnapshotPath(directory))
	assertPrivateFile(t, WALPath(directory))
}

func TestOpenExcludesAnotherRecorderUntilClose(t *testing.T) {
	directory := t.TempDir()
	first, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Open(Options{Directory: directory}); err == nil {
		_ = second.Close()
		_ = first.Close()
		t.Fatal("second recorder unexpectedly acquired an already locked usage directory")
	} else if !errors.Is(err, ErrDirectoryInUse) {
		_ = first.Close()
		t.Fatalf("second Open error = %v, want ErrDirectoryInUse", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUsageDirectoryLockIsReleasedWhenOwnerProcessCrashes(t *testing.T) {
	const helperEnvironment = "COUCHPILOT_USAGE_LOCK_HELPER"
	if os.Getenv(helperEnvironment) == "1" {
		directory := os.Getenv("COUCHPILOT_USAGE_LOCK_DIRECTORY")
		ready := os.Getenv("COUCHPILOT_USAGE_LOCK_READY")
		if _, err := Open(Options{Directory: directory}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		select {}
	}

	directory := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestUsageDirectoryLockIsReleasedWhenOwnerProcessCrashes$")
	var childOutput bytes.Buffer
	command.Stdout = &childOutput
	command.Stderr = &childOutput
	command.Env = append(os.Environ(),
		helperEnvironment+"=1",
		"COUCHPILOT_USAGE_LOCK_DIRECTORY="+directory,
		"COUCHPILOT_USAGE_LOCK_READY="+ready,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	childExited := false
	t.Cleanup(func() {
		if !childExited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lock helper did not become ready: %s", childOutput.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if contender, err := Open(Options{Directory: directory}); err == nil {
		_ = contender.Close()
		t.Fatal("parent acquired usage directory while helper process held it")
	} else if !errors.Is(err, ErrDirectoryInUse) {
		t.Fatalf("contending Open error = %v, want ErrDirectoryInUse", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	childExited = true

	var recorder *FileRecorder
	var err error
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		recorder, err = Open(Options{Directory: directory})
		if err == nil {
			break
		}
		if !errors.Is(err, ErrDirectoryInUse) {
			t.Fatalf("Open after helper crash: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if recorder == nil {
		t.Fatalf("usage lock remained held after helper crash: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecorderAggregatesLifetimeAndLocalDailyCounts(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 7, 20, 0, 30, 0, 0, testLocation)
	recorder, err := Open(Options{
		Directory:     directory,
		Now:           func() time.Time { return now },
		FlushInterval: time.Hour,
		MaxBatchSize:  64,
	})
	if err != nil {
		t.Fatal(err)
	}

	base := Observation{
		ForegroundApp:  "chrome.exe",
		ActiveProfile:  "chrome",
		BindingProfile: "default",
		Control:        "a",
		Gesture:        "a",
		Action:         "click_left",
		Resolution:     ResolutionBound,
	}
	success := base
	success.At = now // This is 2026-07-19 in UTC, but 2026-07-20 locally.
	success.Outcome = OutcomeSuccess
	failure := base
	failure.At = now.AddDate(0, 0, -1)
	failure.Outcome = OutcomeFailure
	withoutTimestamp := base
	withoutTimestamp.Outcome = OutcomeNone

	recorder.Record(success)
	recorder.Record(failure)
	recorder.Record(withoutTimestamp)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	persisted := readPersistedSnapshot(t, directory)
	if persisted.AppliedThrough != 1 {
		t.Fatalf("applied_through = %d, want 1", persisted.AppliedThrough)
	}
	if len(persisted.Entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(persisted.Entries))
	}
	entry := persisted.Entries[0]
	if entry.ForegroundApp != "chrome.exe" {
		t.Fatalf("foreground_app = %q, want chrome.exe", entry.ForegroundApp)
	}
	if entry.Attempts != 3 || entry.Successes != 1 || entry.Failures != 1 {
		t.Fatalf("lifetime counters = %d/%d/%d, want 3/1/1", entry.Attempts, entry.Successes, entry.Failures)
	}
	if entry.FirstUsedDay != "2026-07-19" || entry.LastUsedDay != "2026-07-20" {
		t.Fatalf("first/last = %q/%q", entry.FirstUsedDay, entry.LastUsedDay)
	}
	if got := entry.Daily["2026-07-20"]; got.Attempts != 2 || got.Successes != 1 || got.Failures != 0 {
		t.Fatalf("2026-07-20 counters = %#v", got)
	}
	if got := entry.Daily["2026-07-19"]; got.Attempts != 1 || got.Failures != 1 {
		t.Fatalf("2026-07-19 counters = %#v", got)
	}
	if info, err := os.Stat(WALPath(directory)); err != nil || info.Size() != 0 {
		t.Fatalf("WAL after close: info=%v err=%v", info, err)
	}
}

func TestOpenReplaysNewWALBatchOnceAndDiscardsPartialTail(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	key := entryKey{
		ForegroundApp:  "chrome.exe",
		ActiveProfile:  "chrome",
		BindingProfile: "chrome",
		Control:        "rb",
		Gesture:        "rb",
		Action:         "tab_next",
		Resolution:     ResolutionBound,
	}
	state := aggregateState{
		UpdatedDay:     "2026-07-19",
		AppliedThrough: 1,
		Entries: map[entryKey]*aggregateEntry{
			key: {
				Key:          key,
				Attempts:     2,
				Successes:    2,
				FirstUsedDay: "2026-07-19",
				LastUsedDay:  "2026-07-19",
				Daily:        map[string]counters{"2026-07-19": {Attempts: 2, Successes: 2}},
			},
		},
	}
	writeAggregateSnapshot(t, directory, state)

	duplicate := walBatch{
		SchemaVersion: schemaVersion,
		BatchID:       1,
		Deltas: []walDelta{{
			ForegroundApp: "chrome.exe",
			ActiveProfile: "chrome", BindingProfile: "chrome", Control: "rb", Gesture: "rb",
			Action: "tab_next", Resolution: ResolutionBound, Day: "2026-07-19", Attempts: 2, Successes: 2,
		}},
	}
	newBatch := walBatch{
		SchemaVersion: schemaVersion,
		BatchID:       2,
		Deltas: []walDelta{{
			ForegroundApp: "chrome.exe",
			ActiveProfile: "chrome", BindingProfile: "chrome", Control: "rb", Gesture: "rb",
			Action: "tab_next", Resolution: ResolutionBound, Day: "2026-07-20", Attempts: 1, Successes: 1,
		}},
	}
	wal := append(mustJSON(t, duplicate), '\n')
	wal = append(wal, mustJSON(t, newBatch)...)
	wal = append(wal, '\n')
	wal = append(wal, []byte(`{"schema_version":1,"batch_id":3,"deltas":[`)...)
	if err := os.WriteFile(WALPath(directory), wal, 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 20, 10, 0, 0, 0, testLocation)
	recorder, err := Open(Options{Directory: directory, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	persisted := readPersistedSnapshot(t, directory)
	if persisted.AppliedThrough != 2 {
		t.Fatalf("applied_through = %d, want 2", persisted.AppliedThrough)
	}
	if len(persisted.Entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(persisted.Entries))
	}
	entry := persisted.Entries[0]
	if entry.Attempts != 3 || entry.Successes != 3 {
		t.Fatalf("replayed counters = %d/%d, want 3/3", entry.Attempts, entry.Successes)
	}
	if info, err := os.Stat(WALPath(directory)); err != nil || info.Size() != 0 {
		t.Fatalf("WAL after recovery: info=%v err=%v", info, err)
	}
}

func TestAppendWALBatchRollsBackPartialWriteAndSyncFailure(t *testing.T) {
	newBatch := func(id uint64, control string) walBatch {
		return walBatch{
			SchemaVersion: schemaVersion,
			BatchID:       id,
			Deltas: []walDelta{{
				Kind: EventInputAttempt, Control: control, Gesture: control,
				Resolution: ResolutionObserved, Day: "2026-07-21", Attempts: 1,
			}},
		}
	}
	tests := []struct {
		name       string
		operations func() walAppendIO
	}{
		{
			name: "partial write",
			operations: func() walAppendIO {
				return walAppendIO{
					write: func(file *os.File, data []byte) (int, error) {
						written, err := file.Write(data[:len(data)/2])
						if err != nil {
							return written, err
						}
						return written, errors.New("injected partial write failure")
					},
					sync: func(file *os.File) error { return file.Sync() },
				}
			},
		},
		{
			name: "sync failure",
			operations: func() walAppendIO {
				syncCalls := 0
				return walAppendIO{
					write: func(file *os.File, data []byte) (int, error) { return file.Write(data) },
					sync: func(file *os.File) error {
						syncCalls++
						if syncCalls == 1 {
							return errors.New("injected sync failure")
						}
						return file.Sync()
					},
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := prepareDirectory(directory); err != nil {
				t.Fatal(err)
			}
			if err := appendWALBatch(directory, newBatch(1, "a")); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(WALPath(directory))
			if err != nil {
				t.Fatal(err)
			}
			if err := appendWALBatchWithIO(directory, newBatch(2, "b"), test.operations()); err == nil {
				t.Fatal("fault-injected WAL append unexpectedly succeeded")
			}
			after, err := os.ReadFile(WALPath(directory))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("WAL changed after failed append: before=%d after=%d", len(before), len(after))
			}
			if err := appendWALBatch(directory, newBatch(2, "b")); err != nil {
				t.Fatalf("retry WAL append: %v", err)
			}
			state := emptyAggregate()
			if err := replayWAL(directory, &state); err != nil {
				t.Fatalf("replay after retry: %v", err)
			}
			if state.AppliedThrough != 2 || len(state.Entries) != 2 {
				t.Fatalf("replayed state = batch %d, entries %d", state.AppliedThrough, len(state.Entries))
			}
		})
	}
}

func TestWorkerCompactsFullWALAndRetriesPendingBatch(t *testing.T) {
	directory := t.TempDir()
	recorder, err := Open(Options{
		Directory: directory, FlushInterval: time.Hour, MaxBatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(WALPath(directory), bytes.Repeat([]byte{'x'}, walMaximumSize), 0o600); err != nil {
		_ = recorder.Close()
		t.Fatal(err)
	}
	recorder.Record(Observation{
		Kind: EventInputAttempt, Control: "a", Gesture: "a", Action: "click_left",
		Resolution: ResolutionBound, Outcome: OutcomeSuccess,
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, readErr := os.ReadFile(WALPath(directory))
		if readErr == nil && len(data) > 0 && len(data) < walMaximumSize && bytes.Contains(data, []byte(`"batch_id":1`)) {
			break
		}
		if time.Now().After(deadline) {
			_ = recorder.Close()
			t.Fatalf("worker did not compact and retry the full WAL: size=%d err=%v", len(data), readErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	persisted := readPersistedSnapshot(t, directory)
	var inputAttempts uint64
	for _, entry := range persisted.Entries {
		if entry.Kind == EventInputAttempt && entry.Control == "a" {
			inputAttempts += entry.Attempts
		}
	}
	if persisted.AppliedThrough != 1 || inputAttempts != 1 {
		t.Fatalf("snapshot after full-WAL recovery = %#v", persisted)
	}
}

func TestOpenRecoversCorruptPrimaryFromBackup(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	state := aggregateState{UpdatedDay: "2026-07-19", Entries: make(map[entryKey]*aggregateEntry), Dropped: 7}
	persisted := makeSnapshot(state)
	data, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotBackupPath(directory), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SnapshotPath(directory), []byte(`{"schema_version":`), 0o600); err != nil {
		t.Fatal(err)
	}

	reported := make(chan error, 1)
	recorder, err := Open(Options{
		Directory: directory,
		Now:       func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 0, testLocation) },
		OnError: func(err error) {
			reported <- err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case recoveryErr := <-reported:
		if !strings.Contains(recoveryErr.Error(), "recover usage snapshot") {
			t.Fatalf("unexpected recovery error: %v", recoveryErr)
		}
	default:
		t.Fatal("expected backup recovery to be reported")
	}
	if got := readPersistedSnapshot(t, directory).Dropped; got != 7 {
		t.Fatalf("dropped = %d, want 7 from backup", got)
	}
}

func TestCompactionKeepsBackupCompatibleWithNextWALGeneration(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, testLocation)
	state := emptyAggregate()
	state.AppliedThrough = 7
	if err := compact(directory, &state, now); err != nil {
		t.Fatal(err)
	}
	state.AppliedThrough = 8
	if err := compact(directory, &state, now); err != nil {
		t.Fatal(err)
	}

	backup, err := readSnapshot(snapshotBackupPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if backup.AppliedThrough != 8 {
		t.Fatalf("backup applied_through = %d, want 8", backup.AppliedThrough)
	}
	batch := walBatch{
		SchemaVersion: schemaVersion,
		BatchID:       9,
		Deltas: []walDelta{{
			Control: "a", Gesture: "a", Resolution: ResolutionObserved,
			Day: "2026-07-20", Attempts: 1,
		}},
	}
	if err := appendWALBatch(directory, batch); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SnapshotPath(directory), []byte(`{"schema_version":`), 0o600); err != nil {
		t.Fatal(err)
	}

	recovered, recoveryErr, err := loadAggregate(directory)
	if err != nil {
		t.Fatal(err)
	}
	if recoveryErr == nil {
		t.Fatal("expected primary snapshot recovery to be reported")
	}
	if err := replayWAL(directory, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.AppliedThrough != 9 {
		t.Fatalf("recovered applied_through = %d, want 9", recovered.AppliedThrough)
	}
	key := entryKey{Control: "a", Gesture: "a", Resolution: ResolutionObserved}
	if entry := recovered.Entries[key]; entry == nil || entry.Attempts != 1 {
		t.Fatalf("recovered entry = %#v, want one attempt", entry)
	}
}

func TestSmallWALRefreshesSnapshotAfterOneMinute(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, testLocation)
	state := emptyAggregate()
	if err := compact(directory, &state, now); err != nil {
		t.Fatal(err)
	}
	batch := walBatch{
		SchemaVersion: schemaVersion,
		BatchID:       1,
		Deltas: []walDelta{{
			Control: "a", Gesture: "a", Resolution: ResolutionObserved,
			Day: "2026-07-20", Attempts: 1,
		}},
	}
	if err := appendWALBatch(directory, batch); err != nil {
		t.Fatal(err)
	}
	applyWALBatch(&state, batch)
	recorder := FileRecorder{
		directory:           directory,
		state:               state,
		lastSnapshotRefresh: now,
	}
	if err := recorder.compactIfNeeded(now.Add(snapshotRefreshTime - time.Second)); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(WALPath(directory)); err != nil || info.Size() == 0 {
		t.Fatalf("small WAL compacted too early: info=%v err=%v", info, err)
	}
	if err := recorder.compactIfNeeded(now.Add(snapshotRefreshTime)); err != nil {
		t.Fatal(err)
	}
	persisted := readPersistedSnapshot(t, directory)
	if persisted.AppliedThrough != 1 || len(persisted.Entries) != 1 {
		t.Fatalf("refreshed snapshot = %#v", persisted)
	}
	if info, err := os.Stat(WALPath(directory)); err != nil || info.Size() != 0 {
		t.Fatalf("WAL after periodic snapshot refresh: info=%v err=%v", info, err)
	}
}

func TestCompactionKeepsOnlyNinetyLocalDaysAndRecomputesTotals(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, testLocation)
	key := entryKey{Control: "a", Gesture: "a", Resolution: ResolutionObserved}
	daily := make(map[string]counters)
	for offset := 0; offset < recentDayCount+5; offset++ {
		day := dayString(now.AddDate(0, 0, -offset))
		daily[day] = counters{Attempts: 1}
	}
	state := aggregateState{
		UpdatedDay: "2026-07-19",
		Entries: map[entryKey]*aggregateEntry{
			key: {
				Key: key, Attempts: recentDayCount + 5, FirstUsedDay: dayString(now.AddDate(0, 0, -(recentDayCount + 4))),
				LastUsedDay: dayString(now), Daily: daily,
			},
		},
	}
	writeAggregateSnapshot(t, directory, state)

	recorder, err := Open(Options{Directory: directory, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	entry := readPersistedSnapshot(t, directory).Entries[0]
	if len(entry.Daily) != recentDayCount {
		t.Fatalf("daily bucket count = %d, want %d", len(entry.Daily), recentDayCount)
	}
	if _, found := entry.Daily[dayString(now.AddDate(0, 0, -(recentDayCount-1)))]; !found {
		t.Fatal("oldest retained local day is missing")
	}
	if _, found := entry.Daily[dayString(now.AddDate(0, 0, -recentDayCount))]; found {
		t.Fatal("first expired local day was not pruned")
	}
	if entry.Attempts != recentDayCount || entry.FirstUsedDay != dayString(now.AddDate(0, 0, -(recentDayCount-1))) {
		t.Fatalf("window totals were not recomputed during pruning: %#v", entry)
	}
}

func TestRecordIsNonBlockingAndCountsQueueOverflow(t *testing.T) {
	recorder := &FileRecorder{records: make(chan Observation, 1)}
	recorder.Record(Observation{})

	returned := make(chan struct{})
	go func() {
		recorder.Record(Observation{})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Record blocked on a full queue")
	}
	if got := recorder.dropped.Load(); got != 1 {
		t.Fatalf("dropped = %d, want 1", got)
	}
}

func TestQueueOverflowIsPersistedAsDropped(t *testing.T) {
	directory := t.TempDir()
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	recorder, err := Open(Options{
		Directory:     directory,
		FlushInterval: time.Hour,
		MaxBatchSize:  1,
		OnError: func(error) {
			close(callbackEntered)
			<-releaseCallback
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Observation{Resolution: Resolution("invalid")})
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter error callback")
	}

	valid := Observation{Control: "a", Gesture: "a", Resolution: ResolutionObserved, Outcome: OutcomeNone}
	overflow := 5
	for index := 0; index < cap(recorder.records)+overflow; index++ {
		recorder.Record(valid)
	}
	if got := recorder.dropped.Load(); got != uint64(overflow) {
		t.Fatalf("queued overflow = %d, want %d", got, overflow)
	}
	close(releaseCallback)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	persisted := readPersistedSnapshot(t, directory)
	if persisted.Dropped != uint64(overflow) || persisted.InvalidDropped != 1 {
		t.Fatalf("queue/invalid dropped = %d/%d, want %d/1", persisted.Dropped, persisted.InvalidDropped, overflow)
	}
}

func TestMaxBatchSizeFlushesWALBeforeClose(t *testing.T) {
	directory := t.TempDir()
	exactTime := time.Date(2026, 7, 20, 17, 18, 19, 123456789, testLocation)
	recorder, err := Open(Options{
		Directory:     directory,
		Now:           func() time.Time { return exactTime },
		FlushInterval: time.Hour,
		MaxBatchSize:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Observation{At: exactTime, Control: "a", Gesture: "a", Resolution: ResolutionObserved, Outcome: OutcomeNone})
	deadline := time.Now().Add(2 * time.Second)
	for {
		info, statErr := os.Stat(WALPath(directory))
		if statErr == nil && info.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = recorder.Close()
			t.Fatalf("batch-size flush did not write WAL: %v", statErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	wal, err := os.ReadFile(WALPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	walText := string(wal)
	if !strings.Contains(walText, `"day":"2026-07-20"`) {
		t.Fatalf("WAL does not contain the aggregate day: %s", walText)
	}
	if strings.Contains(walText, `"at"`) || strings.Contains(walText, `"timestamp"`) || strings.Contains(walText, exactTime.Format(time.RFC3339Nano)) {
		t.Fatalf("WAL leaked an exact observation timestamp: %s", walText)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidObservationIsDroppedAndReported(t *testing.T) {
	directory := t.TempDir()
	reported := make(chan error, 1)
	recorder, err := Open(Options{
		Directory:     directory,
		FlushInterval: time.Hour,
		OnError: func(err error) {
			reported <- err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Observation{Resolution: Resolution("secret")})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case report := <-reported:
		if !strings.Contains(report.Error(), "invalid resolution") {
			t.Fatalf("unexpected report: %v", report)
		}
	default:
		t.Fatal("expected invalid observation report")
	}
	persisted := readPersistedSnapshot(t, directory)
	if persisted.Dropped != 0 || persisted.InvalidDropped != 1 {
		t.Fatalf("queue/invalid dropped = %d/%d, want 0/1", persisted.Dropped, persisted.InvalidDropped)
	}
}

func TestOpenRejectsCorruptWALMiddleLine(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(WALPath(directory), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(Options{Directory: directory})
	if err == nil || !strings.Contains(err.Error(), "decode usage WAL") {
		t.Fatalf("Open error = %v, want WAL decode error", err)
	}
	if err := os.Remove(WALPath(directory)); err != nil {
		t.Fatal(err)
	}
	recorder, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatalf("failed Open leaked the usage directory lock: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	recorder, err := Open(Options{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
}

func readPersistedSnapshot(t *testing.T, directory string) snapshot {
	t.Helper()
	data, err := os.ReadFile(SnapshotPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	var persisted snapshot
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	return persisted
}

func writeAggregateSnapshot(t *testing.T, directory string, state aggregateState) {
	t.Helper()
	data, err := json.Marshal(makeSnapshot(state))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SnapshotPath(directory), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not expose ACLs through FileMode. On platforms that do honor
	// permission bits, ensure no group/other access was introduced.
	if info.Mode().Perm()&0o077 != 0 && filepath.Separator != '\\' {
		t.Fatalf("mode for %s = %o, want private", path, info.Mode().Perm())
	}
}

func TestOpenValidatesOptions(t *testing.T) {
	if _, err := Open(Options{}); err == nil {
		t.Fatal("expected empty directory error")
	}
	if _, err := Open(Options{Directory: t.TempDir(), FlushInterval: -time.Second}); err == nil {
		t.Fatal("expected negative flush interval error")
	}
	if _, err := Open(Options{Directory: t.TempDir(), MaxBatchSize: -1}); err == nil {
		t.Fatal("expected negative max batch size error")
	}
	if _, err := Open(Options{
		Directory: t.TempDir(),
		Inventory: []BindingDefinition{{Resolution: Resolution("invalid")}},
	}); err == nil {
		t.Fatal("expected invalid inventory resolution error")
	}
}

func TestReadSnapshotRejectsUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readSnapshot(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Fatalf("readSnapshot error = %v", err)
	}
}

func TestLoadAggregateReturnsErrorWhenBothSnapshotsAreCorrupt(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(SnapshotPath(directory), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotBackupPath(directory), []byte("also bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadAggregate(directory)
	if err == nil {
		t.Fatal("expected corrupt snapshot error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected missing-file classification: %v", err)
	}
}
