//go:build !windows

package daemon

import "errors"

func InstallAutostart(executable, configPath string, verbose bool) error {
	return errors.New("startup installation is currently supported only on Windows")
}

func UninstallAutostart() error {
	return errors.New("startup installation is currently supported only on Windows")
}
