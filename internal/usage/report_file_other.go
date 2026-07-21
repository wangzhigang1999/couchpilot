//go:build !windows

package usage

import "os"

func readReportFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
