package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorderWritesOneNormalizedFactPerLine(t *testing.T) {
	directory := t.TempDir()
	recorder, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 21, 18, 30, 0, 123456789, time.FixedZone("CST", 8*60*60))
	recorder.Record(Fact{
		At: at, Kind: InputAttempt,
		ForegroundApp:   " /Applications/Google Chrome.app/Contents/MacOS/Google Chrome ",
		ActiveProfile:   " chrome ",
		BindingProfile:  " chrome ",
		Control:         " rb ",
		PhysicalGesture: " rb ",
		Gesture:         " rb ",
		Action:          " tab_next ",
		Resolution:      Bound,
		Outcome:         Success,
		Flags:           " simultaneous ",
	})
	recorder.Record(Fact{At: at, Kind: PhysicalActivation, Control: "lt", Resolution: Observed, Outcome: NoOutcome})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	lines := readLines(t, Path(directory))
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), lines)
	}
	var fact Fact
	if err := json.Unmarshal([]byte(lines[0]), &fact); err != nil {
		t.Fatalf("first line is not JSON: %v", err)
	}
	if fact.At.Format(time.RFC3339Nano) != "2026-07-21T10:30:00.123456789Z" {
		t.Fatalf("at=%q", fact.At.Format(time.RFC3339Nano))
	}
	if fact.ForegroundApp != "Google Chrome" || fact.ActiveProfile != "chrome" || fact.Control != "rb" {
		t.Fatalf("fact was not normalized: %+v", fact)
	}
}

func TestRecorderAppendsAcrossOpen(t *testing.T) {
	directory := t.TempDir()
	for _, control := range []string{"a", "b"} {
		recorder, err := Open(Options{Directory: directory})
		if err != nil {
			t.Fatal(err)
		}
		recorder.Record(Fact{Kind: InputAttempt, Control: control})
		if err := recorder.Close(); err != nil {
			t.Fatal(err)
		}
	}
	lines := readLines(t, Path(directory))
	if len(lines) != 2 || !strings.Contains(lines[0], `"control":"a"`) || !strings.Contains(lines[1], `"control":"b"`) {
		t.Fatalf("trace was not appended: %q", lines)
	}
}

func TestOpenTightensExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(directory), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(Path(directory), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(Path(directory))
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("permissions directory=%o file=%o", directoryInfo.Mode().Perm(), fileInfo.Mode().Perm())
	}
}

func TestRecorderConcurrentWritesRemainWholeLines(t *testing.T) {
	directory := t.TempDir()
	recorder, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	const count = 200
	var group sync.WaitGroup
	group.Add(count)
	for index := 0; index < count; index++ {
		go func() {
			defer group.Done()
			recorder.Record(Fact{Kind: InputAttempt, Control: "a", Resolution: Bound, Outcome: Success})
		}()
	}
	group.Wait()
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	lines := readLines(t, Path(directory))
	if len(lines) != count {
		t.Fatalf("got %d lines, want %d", len(lines), count)
	}
	for index, line := range lines {
		var fact Fact
		if err := json.Unmarshal([]byte(line), &fact); err != nil {
			t.Fatalf("line %d is interleaved or invalid: %v: %q", index, err, line)
		}
	}
}

func TestRecorderCreatesOnlyTraceJSONL(t *testing.T) {
	directory := t.TempDir()
	recorder, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Fact{Kind: InputAttempt, Control: "a"})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "trace.jsonl" || entries[0].IsDir() {
		t.Fatalf("unexpected trace files: %+v", entries)
	}
}

func TestRecorderResetsTheSingleFileAtSizeLimit(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(Path(directory), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(Path(directory), maxFileBytes); err != nil {
		t.Fatal(err)
	}
	recorder, err := Open(Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Fact{Kind: InputAttempt, Control: "a", Resolution: Bound, Outcome: Success})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, Path(directory))
	if len(lines) != 1 || !strings.Contains(lines[0], `"control":"a"`) {
		t.Fatalf("reset trace lines = %q", lines)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != fileName {
		t.Fatalf("trace reset created extra files: %+v", entries)
	}
}

func TestRecorderReportsInvalidFactsAndKeepsWriting(t *testing.T) {
	directory := t.TempDir()
	var errorsMu sync.Mutex
	var errorsSeen []error
	recorder, err := Open(Options{
		Directory: directory,
		OnError: func(err error) {
			errorsMu.Lock()
			defer errorsMu.Unlock()
			errorsSeen = append(errorsSeen, err)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(Fact{Kind: "episode", Control: "a"})
	recorder.Record(Fact{Kind: InputAttempt, Control: " "})
	recorder.Record(Fact{Kind: InputAttempt, Control: "a"})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	if len(errorsSeen) != 2 {
		t.Fatalf("got %d validation errors, want 2", len(errorsSeen))
	}
	if lines := readLines(t, Path(directory)); len(lines) != 1 {
		t.Fatalf("got %d valid lines, want 1", len(lines))
	}
}

func TestRecorderCloseIsIdempotent(t *testing.T) {
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
	recorder.Record(Fact{Kind: InputAttempt, Control: "a"})
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}
