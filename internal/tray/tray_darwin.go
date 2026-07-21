//go:build darwin

package tray

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include "tray_darwin.h"
*/
import "C"

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"unsafe"
)

//go:embed assets/couchpilot-menubar.svg
var menuBarIcon []byte

type darwinApplication struct {
	cancel          context.CancelFunc
	configDirectory string
	logDirectory    string
	closeOnce       sync.Once
}

var (
	darwinTrayMu     sync.Mutex
	activeDarwinTray *darwinApplication
)

// New creates the status item synchronously on the process main thread. The
// returned application then owns AppKit's standard main loop in this process.
func New(cancel context.CancelFunc, options Options) (Application, error) {
	darwinTrayMu.Lock()
	defer darwinTrayMu.Unlock()
	if activeDarwinTray != nil {
		return nil, fmt.Errorf("macOS menu bar icon is already running")
	}
	if len(menuBarIcon) == 0 {
		return nil, fmt.Errorf("macOS menu bar icon asset is empty")
	}
	result := C.cp_tray_start(
		(*C.uchar)(unsafe.Pointer(&menuBarIcon[0])),
		C.int(len(menuBarIcon)),
	)
	if result != 0 {
		return nil, fmt.Errorf("create macOS menu bar icon: native result %d", int(result))
	}
	application := &darwinApplication{
		cancel:          cancel,
		configDirectory: filepath.Dir(options.ConfigPath),
		logDirectory:    filepath.Dir(options.LogPath),
	}
	activeDarwinTray = application
	return application, nil
}

func (t *darwinApplication) Run(ctx context.Context) error {
	defer t.Close()
	watcherDone := make(chan struct{})
	watcherExited := make(chan struct{})
	go func() {
		defer close(watcherExited)
		select {
		case <-ctx.Done():
			C.cp_tray_stop()
		case <-watcherDone:
		}
	}()
	result := C.cp_tray_run_main_loop()
	close(watcherDone)
	<-watcherExited
	if result != 0 {
		t.cancel()
		return fmt.Errorf("run macOS menu bar event loop: native result %d", int(result))
	}
	return nil
}

func (t *darwinApplication) Close() error {
	t.closeOnce.Do(func() {
		C.cp_tray_dispose()
		darwinTrayMu.Lock()
		if activeDarwinTray == t {
			activeDarwinTray = nil
		}
		darwinTrayMu.Unlock()
	})
	return nil
}

//export couchpilotTrayCommand
func couchpilotTrayCommand(command C.int) {
	darwinTrayMu.Lock()
	tray := activeDarwinTray
	darwinTrayMu.Unlock()
	if tray == nil {
		return
	}
	var err error
	switch int(command) {
	case 1:
		err = openDarwinDirectory(tray.logDirectory)
	case 2:
		err = openDarwinDirectory(tray.configDirectory)
	case 3:
		tray.cancel()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "menu bar:", err)
	}
}

func openDarwinDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	if err := startDarwinOpen(path); err != nil {
		return fmt.Errorf("open directory %s: %w", path, err)
	}
	return nil
}

func startDarwinOpen(path string) error {
	command := exec.Command("open", path)
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
