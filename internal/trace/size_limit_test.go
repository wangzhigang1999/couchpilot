package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOversizedFactPreservesExistingTrace(t *testing.T) {
	for _, test := range []struct {
		name    string
		control string
	}{
		{"large text", strings.Repeat("a", maxFileBytes)},
		{"JSON escaping expands text", strings.Repeat("<", maxFileBytes/5)},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			var failures []error
			recorder, err := Open(Options{Directory: directory, OnError: func(err error) {
				failures = append(failures, err)
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { recorder.Close() })
			recorder.Record(Fact{Kind: InputAttempt, Control: "before"})
			before, err := os.ReadFile(Path(directory))
			if err != nil {
				t.Fatal(err)
			}
			recorder.Record(Fact{Kind: InputAttempt, Control: test.control})
			if len(failures) != 1 || !strings.Contains(failures[0].Error(), "exceeds") {
				t.Errorf("expected one record-size error, got %v", failures)
			}
			after, err := os.ReadFile(Path(directory))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || recorder.size != int64(len(before)) {
				t.Errorf("oversized fact changed trace: before=%d after=%d tracked=%d", len(before), len(after), recorder.size)
			}
			recorder.Record(Fact{Kind: InputAttempt, Control: "after"})
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}
			lines := readLines(t, Path(directory))
			if len(lines) != 2 || !strings.Contains(lines[0], `"control":"before"`) || !strings.Contains(lines[1], `"control":"after"`) {
				t.Errorf("recorder did not preserve and resume normal records: %d lines", len(lines))
			}
		})
	}
}

func TestFactExactlyAtFileLimitIsAccepted(t *testing.T) {
	directory := t.TempDir()
	recorder, err := Open(Options{Directory: directory, OnError: func(err error) { t.Error(err) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { recorder.Close() })
	fact := Fact{At: time.Unix(100, 0).UTC(), Kind: InputAttempt, Control: "a"}
	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	// Replace the one-byte control and reserve one byte for the JSONL newline.
	fact.Control = strings.Repeat("a", maxFileBytes-len(encoded))
	recorder.Record(fact)
	info, err := os.Stat(Path(directory))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != maxFileBytes || recorder.size != maxFileBytes {
		t.Fatalf("exact-limit record size: file=%d tracked=%d", info.Size(), recorder.size)
	}
	recorder.Record(Fact{Kind: InputAttempt, Control: "next"})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, Path(directory))
	if len(lines) != 1 || !strings.Contains(lines[0], `"control":"next"`) {
		t.Fatal("next valid record did not reset the full trace normally")
	}
}
