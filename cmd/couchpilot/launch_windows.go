//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	consoleKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procFreeConsole           = consoleKernel32.NewProc("FreeConsole")
	procGetConsoleProcessList = consoleKernel32.NewProc("GetConsoleProcessList")
)

// prepareLaunchArgs keeps the normal foreground CLI when CouchPilot is run
// from a terminal. A no-argument Explorer launch owns a brand-new console by
// itself; detach that console and use the regular background start path so a
// double-click produces only the notification-area icon.
func prepareLaunchArgs(args []string) []string {
	if len(args) != 0 || !standaloneConsole() {
		return args
	}
	detached, _, _ := procFreeConsole.Call()
	if detached == 0 {
		return args
	}
	return []string{"start", "--config", explorerConfigPath()}
}

func standaloneConsole() bool {
	var processIDs [2]uint32
	count, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&processIDs[0])),
		uintptr(len(processIDs)),
	)
	runtime.KeepAlive(&processIDs)
	return count == 1
}

func explorerConfigPath() string {
	executable, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return configPathForExecutable(executable)
}

func configPathForExecutable(executable string) string {
	executableDirectory := filepath.Dir(executable)
	candidates := []string{
		filepath.Join(executableDirectory, "config.json"),
		filepath.Join(filepath.Dir(executableDirectory), "config.json"),
	}
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	// A portable first run creates its config beside the executable.
	return candidates[0]
}
