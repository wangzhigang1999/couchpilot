package trace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	fileName     = "trace.jsonl"
	maxFileBytes = 8 << 20
)

type Kind string

const (
	InputAttempt       Kind = "input_attempt"
	PhysicalActivation Kind = "physical_activation"
)

type Resolution string

const (
	Bound    Resolution = "bound"
	Disabled Resolution = "disabled"
	Unbound  Resolution = "unbound"
	Observed Resolution = "observed"
	System   Resolution = "system"
)

type Outcome string

const (
	NoOutcome Outcome = "none"
	Success   Outcome = "success"
	Failure   Outcome = "failure"
)

// Fact is one immutable trace line. time.Time's JSON representation is
// RFC3339Nano, so no separate wire or persistence model is needed.
type Fact struct {
	At              time.Time  `json:"at"`
	Kind            Kind       `json:"kind"`
	ForegroundApp   string     `json:"foreground_app,omitempty"`
	ActiveProfile   string     `json:"active_profile,omitempty"`
	BindingProfile  string     `json:"binding_profile,omitempty"`
	Control         string     `json:"control"`
	PhysicalGesture string     `json:"physical_gesture,omitempty"`
	Gesture         string     `json:"gesture,omitempty"`
	Action          string     `json:"action,omitempty"`
	Resolution      Resolution `json:"resolution,omitempty"`
	Outcome         Outcome    `json:"outcome,omitempty"`
	Flags           string     `json:"flags,omitempty"`
}

// Sink lets the engine emit facts without owning the recorder lifecycle.
type Sink interface {
	Record(Fact)
}

type Options struct {
	Directory string
	OnError   func(error)
}

// Recorder appends one JSON object per line. It has no buffering, background
// work, rotation, aggregation, or recovery protocol.
type Recorder struct {
	mu      sync.Mutex
	file    *os.File
	onError func(error)
	size    int64
	closed  bool
}

func Path(directory string) string {
	return filepath.Join(directory, fileName)
}

func Open(options Options) (*Recorder, error) {
	directory := strings.TrimSpace(options.Directory)
	if directory == "" {
		return nil, errors.New("trace directory cannot be empty")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create trace directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure trace directory: %w", err)
	}
	file, err := os.OpenFile(Path(directory), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open trace: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure trace file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat trace: %w", err)
	}
	return &Recorder{file: file, onError: options.OnError, size: info.Size()}, nil
}

func (r *Recorder) Record(fact Fact) {
	if r == nil {
		return
	}
	if err := normalize(&fact); err != nil {
		r.report(err)
		return
	}
	line, err := json.Marshal(fact)
	if err != nil {
		r.report(fmt.Errorf("encode trace fact: %w", err))
		return
	}
	line = append(line, '\n')

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	if r.size+int64(len(line)) > maxFileBytes {
		if err := r.file.Truncate(0); err != nil {
			r.mu.Unlock()
			r.report(fmt.Errorf("reset full trace: %w", err))
			return
		}
		r.size = 0
	}
	written, err := r.file.Write(line)
	r.size += int64(written)
	r.mu.Unlock()
	if err == nil && written != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		r.report(fmt.Errorf("append trace fact: %w", err))
	}
}

func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if err := r.file.Close(); err != nil {
		return fmt.Errorf("close trace: %w", err)
	}
	return nil
}

func normalize(fact *Fact) error {
	if fact.Kind != InputAttempt && fact.Kind != PhysicalActivation {
		return fmt.Errorf("unsupported trace kind %q", fact.Kind)
	}
	fact.Control = strings.TrimSpace(fact.Control)
	if fact.Control == "" {
		return errors.New("trace control cannot be empty")
	}
	if fact.At.IsZero() {
		fact.At = time.Now().UTC()
	} else {
		fact.At = fact.At.UTC()
	}
	fact.ForegroundApp = strings.TrimSpace(fact.ForegroundApp)
	if fact.ForegroundApp != "" {
		fact.ForegroundApp = filepath.Base(fact.ForegroundApp)
	}
	fact.ActiveProfile = strings.TrimSpace(fact.ActiveProfile)
	fact.BindingProfile = strings.TrimSpace(fact.BindingProfile)
	fact.PhysicalGesture = strings.TrimSpace(fact.PhysicalGesture)
	fact.Gesture = strings.TrimSpace(fact.Gesture)
	fact.Action = strings.TrimSpace(fact.Action)
	fact.Flags = strings.TrimSpace(fact.Flags)
	return nil
}

func (r *Recorder) report(err error) {
	if r.onError != nil {
		r.onError(err)
	}
}
