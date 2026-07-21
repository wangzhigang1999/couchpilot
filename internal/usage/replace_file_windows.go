//go:build windows

package usage

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const replaceFileAttempts = 12

func removeForReplace(path string) error {
	return retryReplaceOperation(func() error { return os.Remove(path) })
}

func renameForReplace(oldPath, newPath string) error {
	return retryReplaceOperation(func() error { return os.Rename(oldPath, newPath) })
}

// Windows programs commonly open local files without FILE_SHARE_DELETE. Such
// handles make an otherwise atomic remove or rename fail transiently while a
// browser, virus scanner, or report reader finishes its read. Keep replacement
// bounded and retry only the two errors that Windows uses for this condition;
// other errors fail immediately, and persistent access failures still return
// after the bounded retry window.
func retryReplaceOperation(operation func() error) error {
	delay := 2 * time.Millisecond
	for attempt := 0; attempt < replaceFileAttempts; attempt++ {
		err := operation()
		if err == nil || !isTransientReplaceError(err) || attempt+1 == replaceFileAttempts {
			return err
		}
		time.Sleep(delay)
		if delay < 25*time.Millisecond {
			delay *= 2
			if delay > 25*time.Millisecond {
				delay = 25 * time.Millisecond
			}
		}
	}
	return nil
}

func isTransientReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
