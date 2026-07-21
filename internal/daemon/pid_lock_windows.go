//go:build windows

package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

type namedMutexResult struct {
	release func()
	err     error
}

// claimPIDLock owns the named mutex on a dedicated, pinned OS thread because
// Windows mutex ownership is thread-affine. The kernel releases the mutex if
// the process exits, so a crash cannot leave a stale lock behind.
func claimPIDLock(path string) (func(), error) {
	canonical, err := canonicalPIDPath(path)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.ToSlash(canonical))))
	name := `Local\CouchPilot-PID-` + hex.EncodeToString(digest[:])
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode process mutex name: %w", err)
	}

	ready := make(chan namedMutexResult, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		handle, createErr := windows.CreateMutex(nil, true, nameUTF16)
		if errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			ready <- namedMutexResult{err: errPIDLockHeld}
			return
		}
		if createErr != nil {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			ready <- namedMutexResult{err: createErr}
			return
		}

		releaseRequest := make(chan struct{})
		released := make(chan struct{})
		var once sync.Once
		ready <- namedMutexResult{release: func() {
			once.Do(func() { close(releaseRequest) })
			<-released
		}}
		<-releaseRequest
		_ = windows.ReleaseMutex(handle)
		_ = windows.CloseHandle(handle)
		close(released)
	}()

	result := <-ready
	return result.release, result.err
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
