//go:build darwin

package main

import "runtime"

func init() {
	// AppKit requires status-item creation and its event loop on the process's
	// initial OS thread. The controller engine runs independently as a worker.
	runtime.LockOSThread()
}
