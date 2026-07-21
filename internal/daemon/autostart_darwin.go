//go:build darwin

package daemon

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const launchAgentLabel = "io.github.wangzhigang1999.couchpilot"

func InstallAutostart(executable, configPath string, verbose bool) error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	paths := RuntimePaths(configPath)
	if err := os.MkdirAll(filepath.Dir(paths.LogFile), 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	if _, running := Status(paths.PIDFile); running {
		if _, err := Stop(paths); err != nil {
			return fmt.Errorf("stop existing process: %w", err)
		}
	}
	content, err := launchAgentPlist(executable, configPath, verbose)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return err
	}
	target := "gui/" + strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", target+"/"+launchAgentLabel).Run()
	if output, err := exec.Command("launchctl", "bootstrap", target, plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("load launch agent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", target+"/"+launchAgentLabel).CombinedOutput(); err != nil {
		return fmt.Errorf("start launch agent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func UninstallAutostart() error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + launchAgentLabel
	output, bootoutErr := exec.Command("launchctl", "bootout", target).CombinedOutput()
	if bootoutErr != nil && !strings.Contains(string(output), "Could not find service") {
		return fmt.Errorf("unload launch agent: %w: %s", bootoutErr, strings.TrimSpace(string(output)))
	}
	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

func launchAgentPlist(executable, configPath string, verbose bool) (string, error) {
	escape := func(value string) (string, error) {
		var buffer bytes.Buffer
		if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
			return "", err
		}
		return buffer.String(), nil
	}
	arguments := []string{"run", "--config", configPath}
	paths := RuntimePaths(configPath)
	arguments = append(arguments, "--pid-file", paths.PIDFile, "--stop-file", paths.StopFile)
	if verbose {
		arguments = append(arguments, "--verbose")
	}
	items := make([]string, 0, len(arguments)+1)
	for _, value := range append([]string{executable}, arguments...) {
		escaped, err := escape(value)
		if err != nil {
			return "", err
		}
		items = append(items, "    <string>"+escaped+"</string>")
	}
	stdout, err := escape(paths.LogFile)
	if err != nil {
		return "", err
	}
	stderr, err := escape(paths.ErrFile)
	if err != nil {
		return "", err
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + launchAgentLabel + `</string>
  <key>ProgramArguments</key>
  <array>
` + strings.Join(items, "\n") + `
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>` + stdout + `</string>
  <key>StandardErrorPath</key><string>` + stderr + `</string>
</dict>
</plist>
`, nil
}
