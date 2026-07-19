//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

func InstallAutostart(executable, configPath string, verbose bool) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}
	taskXML, err := startupTaskXML(currentUser.Username, executable, configPath, verbose)
	if err != nil {
		return err
	}
	taskFile, err := os.CreateTemp("", "couchpilot-task-*.xml")
	if err != nil {
		return err
	}
	taskPath := taskFile.Name()
	defer os.Remove(taskPath)
	if _, err := taskFile.WriteString(taskXML); err != nil {
		taskFile.Close()
		return err
	}
	if err := taskFile.Close(); err != nil {
		return err
	}
	if output, err := exec.Command("schtasks.exe", "/Create", "/TN", startupTaskName, "/XML", taskPath, "/F").CombinedOutput(); err != nil {
		return fmt.Errorf("create startup task: %w: %s", err, strings.TrimSpace(string(output)))
	}
	paths := RuntimePaths(configPath)
	if _, running := Status(paths.PIDFile); running {
		if _, err := Stop(paths); err != nil {
			return fmt.Errorf("stop existing process: %w", err)
		}
	}
	if output, err := exec.Command("schtasks.exe", "/Run", "/TN", startupTaskName).CombinedOutput(); err != nil {
		return fmt.Errorf("start scheduled task: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func UninstallAutostart() error {
	output, err := exec.Command("schtasks.exe", "/Delete", "/TN", startupTaskName, "/F").CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(message, "cannot find") || strings.Contains(message, "找不到") {
			return nil
		}
		return fmt.Errorf("delete startup task: %w: %s", err, message)
	}
	return nil
}
