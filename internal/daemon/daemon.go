package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errPIDLockHeld = errors.New("process lock is already held")

type Paths struct {
	PIDFile        string
	StopFile       string
	LogFile        string
	ErrFile        string
	TraceDirectory string
}

// PIDClaim holds the single-instance lock separately from publishing runtime
// readiness. A launcher sees the PID file only after MarkReady succeeds.
type PIDClaim struct {
	path        string
	value       string
	releaseLock func()
	ready       bool
	releaseOnce sync.Once
}

func RuntimePaths(configPath string) Paths {
	base := filepath.Dir(configPath)
	return Paths{
		PIDFile:        filepath.Join(base, "couchpilot.pid"),
		StopFile:       filepath.Join(base, "couchpilot.stop"),
		LogFile:        filepath.Join(base, "logs", "couchpilot.log"),
		ErrFile:        filepath.Join(base, "logs", "couchpilot.err.log"),
		TraceDirectory: filepath.Join(base, "trace"),
	}
}

func Start(executable, configPath string, verbose bool) (int, error) {
	paths := RuntimePaths(configPath)
	if pid, running := Status(paths.PIDFile); running {
		return pid, fmt.Errorf("already running with pid %d", pid)
	}
	if err := os.MkdirAll(filepath.Dir(paths.LogFile), 0o755); err != nil {
		return 0, err
	}
	_ = os.Remove(paths.StopFile)
	stdout, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(paths.ErrFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer stderr.Close()
	args := []string{"run", "--config", configPath, "--pid-file", paths.PIDFile, "--stop-file", paths.StopFile}
	if verbose {
		args = append(args, "--verbose")
	}
	command := exec.Command(executable, args...)
	command.Dir = filepath.Dir(configPath)
	command.Stdout = stdout
	command.Stderr = stderr
	configureDetached(command)
	if err := command.Start(); err != nil {
		return 0, err
	}
	pid := command.Process.Pid
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-exited:
			removePIDIfMatches(paths.PIDFile, pid)
			return 0, fmt.Errorf("background process exited; see %s", paths.ErrFile)
		case <-ticker.C:
			if claimedPID, running := Status(paths.PIDFile); running {
				return claimedPID, nil
			}
		case <-timeout.C:
			_ = command.Process.Kill()
			<-exited
			removePIDIfMatches(paths.PIDFile, pid)
			return 0, fmt.Errorf("background process did not become ready; see %s", paths.ErrFile)
		}
	}
}

func Stop(paths Paths) (int, error) {
	pid, running := Status(paths.PIDFile)
	if !running {
		cleaned, err := cleanupRuntimeFilesIfUnlocked(paths, 0)
		if err != nil {
			return 0, err
		}
		if !cleaned {
			return 0, errors.New("CouchPilot is starting; its runtime lock is held but readiness is not published")
		}
		return 0, nil
	}
	stopRequest := StopRequestPath(paths.StopFile, pid)
	if err := os.WriteFile(stopRequest, []byte("stop\n"), 0o644); err != nil {
		return 0, err
	}
	if cleaned, err := waitAndCleanupRuntimeFiles(paths, pid, 2500*time.Millisecond); err != nil {
		return 0, err
	} else if cleaned {
		return pid, nil
	}
	currentPID, stillOwned := Status(paths.PIDFile)
	if !stillOwned || currentPID != pid {
		return 0, fmt.Errorf("process %d did not stop, but CouchPilot no longer owns its runtime lock", pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, err
	}
	if err := process.Kill(); err != nil {
		return 0, err
	}
	if cleaned, err := waitAndCleanupRuntimeFiles(paths, pid, 2500*time.Millisecond); err != nil {
		return 0, err
	} else if !cleaned {
		return 0, fmt.Errorf("process %d was killed but did not release the CouchPilot runtime lock", pid)
	}
	return pid, nil
}

// StopRequestPath scopes a shutdown request to one published process
// generation, so an old stop command cannot cancel a replacement instance.
func StopRequestPath(base string, pid int) string {
	return base + "." + strconv.Itoa(pid)
}

func waitAndCleanupRuntimeFiles(paths Paths, pid int, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		cleaned, err := cleanupRuntimeFilesIfUnlocked(paths, pid)
		if err != nil || cleaned {
			return cleaned, err
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func cleanupRuntimeFilesIfUnlocked(paths Paths, pid int) (bool, error) {
	if _, err := os.Stat(filepath.Dir(paths.PIDFile)); errors.Is(err, os.ErrNotExist) {
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect runtime directory: %w", err)
	}
	release, err := claimPIDLock(paths.PIDFile)
	if errors.Is(err, errPIDLockHeld) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim runtime lock for cleanup: %w", err)
	}
	defer release()
	files := []string{paths.PIDFile, paths.StopFile}
	if pid > 0 {
		files = append(files, StopRequestPath(paths.StopFile, pid))
	}
	for _, path := range files {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, fmt.Errorf("remove runtime file %s: %w", path, err)
		}
	}
	return true, nil
}

func Status(pidFile string) (int, bool) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 || !processRunning(pid) {
		return 0, false
	}
	held, err := pidLockHeld(pidFile)
	if err != nil || !held {
		return 0, false
	}
	return pid, true
}

func pidLockHeld(path string) (bool, error) {
	release, err := claimPIDLock(path)
	if errors.Is(err, errPIDLockHeld) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	release()
	return false, nil
}

func removePIDIfMatches(path string, pid int) {
	data, err := os.ReadFile(path)
	if err == nil && strings.TrimSpace(string(data)) == strconv.Itoa(pid) {
		_ = os.Remove(path)
	}
}

func ReservePID(path string) (*PIDClaim, error) {
	if path == "" {
		return &PIDClaim{releaseLock: func() {}}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	releaseLock, err := claimPIDLock(path)
	if err != nil {
		if errors.Is(err, errPIDLockHeld) {
			if pid, running := Status(path); running && pid != os.Getpid() {
				return nil, fmt.Errorf("another instance is running with pid %d", pid)
			}
			return nil, errors.New("another instance is already starting or running")
		}
		return nil, fmt.Errorf("claim process lock: %w", err)
	}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			releaseLock()
		}
	}()
	// Owning the kernel lock proves that no live CouchPilot instance owns this
	// runtime path. A leftover PID may now refer to an unrelated reused PID.
	_ = os.Remove(path)
	releaseOnError = false
	return &PIDClaim{
		path:        path,
		value:       strconv.Itoa(os.Getpid()),
		releaseLock: releaseLock,
	}, nil
}

func (c *PIDClaim) MarkReady() error {
	if c == nil || c.path == "" || c.ready {
		return nil
	}
	if err := os.WriteFile(c.path, []byte(c.value+"\n"), 0o644); err != nil {
		return err
	}
	c.ready = true
	return nil
}

func (c *PIDClaim) Release() {
	if c == nil {
		return
	}
	c.releaseOnce.Do(func() {
		if c.ready {
			pid, _ := strconv.Atoi(c.value)
			removePIDIfMatches(c.path, pid)
		}
		c.releaseLock()
	})
}

// ClaimPID preserves the original one-step API for callers that do not need a
// distinct initialization phase.
func ClaimPID(path string) (func(), error) {
	claim, err := ReservePID(path)
	if err != nil {
		return nil, err
	}
	if err := claim.MarkReady(); err != nil {
		claim.Release()
		return nil, err
	}
	return claim.Release, nil
}
