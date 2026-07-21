//go:build !windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

func claimPIDLock(path string) (func(), error) {
	canonical, err := canonicalPIDPath(path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(canonical+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open process lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errPIDLockHeld
		}
		return nil, fmt.Errorf("lock process file: %w", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
			_ = file.Close()
		})
	}, nil
}

func canonicalPIDPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve pid path: %w", err)
	}
	parent := filepath.Dir(absolute)
	if resolved, resolveErr := filepath.EvalSymlinks(parent); resolveErr == nil {
		absolute = filepath.Join(resolved, filepath.Base(absolute))
	}
	return filepath.Clean(absolute), nil
}
