//go:build windows

package usage

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// readReportFile keeps reporting strictly read-only without blocking the
// recorder's atomic snapshot replacement. os.ReadFile opens Windows files
// without FILE_SHARE_DELETE, so even a short report read can otherwise make a
// concurrent rename fail with ERROR_SHARING_VIOLATION.
func readReportFile(path string) ([]byte, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, &os.PathError{Op: "open", Path: path, Err: windows.ERROR_INVALID_HANDLE}
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, &os.PathError{Op: "read", Path: path, Err: err}
	}
	return data, nil
}
