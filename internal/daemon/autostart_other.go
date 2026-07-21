//go:build !windows && !darwin

package daemon

import "errors"

func InstallAutostart(executable, configPath string, verbose bool) error {
	return errors.New("startup installation is supported only on Windows and macOS")
}

func UninstallAutostart() error {
	return errors.New("startup removal is supported only on Windows and macOS")
}
