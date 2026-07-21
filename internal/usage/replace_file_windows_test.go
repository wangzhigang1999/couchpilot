//go:build windows

package usage

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestCompactRetriesWhileSnapshotBackupIsBeingRead(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	state := emptyAggregate()
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, testLocation)
	if err := compact(directory, &state, now); err != nil {
		t.Fatal(err)
	}

	release := holdWithoutDeleteSharing(t, snapshotBackupPath(directory), 35*time.Millisecond)
	batch := walBatch{
		SchemaVersion: schemaVersion,
		BatchID:       1,
		Deltas: []walDelta{{
			Control: "a", Gesture: "a", Resolution: ResolutionObserved,
			Day: "2026-07-21", Attempts: 1,
		}},
	}
	applyWALBatch(&state, batch)
	err := compact(directory, &state, now)
	<-release
	if err != nil {
		t.Fatal(err)
	}

	primary, err := readSnapshot(SnapshotPath(directory))
	if err != nil || primary.AppliedThrough != 1 {
		t.Fatalf("primary snapshot = %#v, err=%v", primary, err)
	}
	backup, err := readSnapshot(snapshotBackupPath(directory))
	if err != nil || backup.AppliedThrough != 1 {
		t.Fatalf("backup snapshot = %#v, err=%v", backup, err)
	}
}

func TestReplaceReportRetriesWhileBrowserIsReading(t *testing.T) {
	path := ReportPath(t.TempDir())
	if err := replaceReport(path, []byte("old report")); err != nil {
		t.Fatal(err)
	}

	release := holdWithoutDeleteSharing(t, path, 35*time.Millisecond)
	err := replaceReport(path, []byte("new report"))
	<-release
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new report" {
		t.Fatalf("report = %q", data)
	}
}

func holdWithoutDeleteSharing(t *testing.T, path string, duration time.Duration) <-chan struct{} {
	t.Helper()
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(duration)
		_ = windows.CloseHandle(handle)
		close(released)
	}()
	return released
}
