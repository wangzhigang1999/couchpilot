//go:build !windows

package usage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

func acquireStoreLock(directory string) (func() error, error) {
	path := filepath.Join(directory, ".usage.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open usage directory lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("%w: %s", ErrDirectoryInUse, directory)
		}
		return nil, fmt.Errorf("lock usage directory: %w", err)
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
			closeErr := file.Close()
			switch {
			case unlockErr != nil && closeErr != nil:
				releaseErr = errors.Join(unlockErr, closeErr)
			case unlockErr != nil:
				releaseErr = unlockErr
			default:
				releaseErr = closeErr
			}
		})
		return releaseErr
	}, nil
}
