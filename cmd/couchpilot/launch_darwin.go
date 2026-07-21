//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wangzhigang1999/couchpilot/internal/daemon"
	"golang.org/x/sys/unix"
)

// A Finder-launched app has no useful working directory. Keep its generated
// configuration and runtime files in the standard per-user location. CLI
// invocations with explicit arguments retain the portable config behavior.
func prepareLaunchArgs(args []string) []string {
	if len(args) == 1 && strings.HasPrefix(args[0], "-psn_") {
		args = nil
	}
	if len(args) != 0 {
		return args
	}
	executable, err := os.Executable()
	if err != nil {
		return args
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return args
	}
	return darwinLaunchArgs(args, executable, home)
}

func darwinLaunchArgs(args []string, executable, home string) []string {
	if len(args) != 0 || !strings.Contains(filepath.ToSlash(executable), ".app/Contents/MacOS/") {
		return args
	}
	configPath := filepath.Join(home, "Library", "Application Support", "CouchPilot", "config.json")
	return []string{"run", "--config", configPath, "--app-launch"}
}

func redirectAppOutput(configPath string, appLaunch bool) error {
	if !appLaunch {
		return nil
	}
	paths := daemon.RuntimePaths(configPath)
	if err := os.MkdirAll(filepath.Dir(paths.LogFile), 0o755); err != nil {
		return fmt.Errorf("create application log directory: %w", err)
	}
	stdout, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open application log: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(paths.ErrFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open application error log: %w", err)
	}
	defer stderr.Close()
	if err := unix.Dup2(int(stdout.Fd()), int(os.Stdout.Fd())); err != nil {
		return fmt.Errorf("redirect application output: %w", err)
	}
	if err := unix.Dup2(int(stderr.Fd()), int(os.Stderr.Fd())); err != nil {
		return fmt.Errorf("redirect application errors: %w", err)
	}
	return nil
}
