//go:build windows

package daemon

import (
	"os/exec"
	"syscall"

	winapi "golang.org/x/sys/windows"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	createNoWindow        = 0x08000000
)

func configureDetached(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNewProcessGroup | detachedProcess | createNoWindow,
	}
}

func processRunning(pid int) bool {
	handle, err := winapi.OpenProcess(winapi.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer winapi.CloseHandle(handle)
	status, err := winapi.WaitForSingleObject(handle, 0)
	return err == nil && status == uint32(winapi.WAIT_TIMEOUT)
}
