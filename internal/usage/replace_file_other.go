//go:build !windows

package usage

import "os"

func removeForReplace(path string) error {
	return os.Remove(path)
}

func renameForReplace(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
