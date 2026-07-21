//go:build windows

package usage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

func acquireStoreLock(directory string) (func() error, error) {
	path := filepath.Join(directory, ".usage.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open usage directory lock: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("protect usage directory lock: %w", err)
	}
	overlapped := &windows.Overlapped{}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, fmt.Errorf("%w: %s", ErrDirectoryInUse, directory)
		}
		return nil, fmt.Errorf("lock usage directory: %w", err)
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			unlockErr := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped)
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
