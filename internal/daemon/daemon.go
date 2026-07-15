package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Paths struct {
	PIDFile  string
	StopFile string
	LogFile  string
	ErrFile  string
}

func RuntimePaths(configPath string) Paths {
	base := filepath.Dir(configPath)
	return Paths{
		PIDFile:  filepath.Join(base, "couchpilot.pid"),
		StopFile: filepath.Join(base, "couchpilot.stop"),
		LogFile:  filepath.Join(base, "logs", "couchpilot.log"),
		ErrFile:  filepath.Join(base, "logs", "couchpilot.err.log"),
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
	if err := command.Process.Release(); err != nil {
		return 0, err
	}
	time.Sleep(250 * time.Millisecond)
	if _, running := Status(paths.PIDFile); !running {
		return 0, fmt.Errorf("background process exited; see %s", paths.ErrFile)
	}
	return pid, nil
}

func Stop(paths Paths) (int, error) {
	pid, running := Status(paths.PIDFile)
	if !running {
		_ = os.Remove(paths.PIDFile)
		_ = os.Remove(paths.StopFile)
		return 0, nil
	}
	if err := os.WriteFile(paths.StopFile, []byte("stop\n"), 0o644); err != nil {
		return 0, err
	}
	for attempt := 0; attempt < 50; attempt++ {
		if !processRunning(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if processRunning(pid) {
		process, err := os.FindProcess(pid)
		if err != nil {
			return 0, err
		}
		if err := process.Kill(); err != nil {
			return 0, err
		}
	}
	_ = os.Remove(paths.PIDFile)
	_ = os.Remove(paths.StopFile)
	return pid, nil
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
	return pid, true
}

func ClaimPID(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	if pid, running := Status(path); running && pid != os.Getpid() {
		return nil, fmt.Errorf("another instance is running with pid %d", pid)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	value := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
		return nil, err
	}
	return func() {
		data, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(data)) == value {
			_ = os.Remove(path)
		}
	}, nil
}
