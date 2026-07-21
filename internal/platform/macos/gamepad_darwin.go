//go:build darwin

package macplatform

/*
#cgo LDFLAGS: -framework Foundation -framework GameController -framework AppKit -framework CoreGraphics -framework IOKit
#include <stdlib.h>
#include "native.h"
*/
import "C"

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type Gamepad struct {
	packet atomic.Uint32
	mu     sync.RWMutex
	known  map[core.DeviceID]gamepadDevice
}

const (
	gamepadBackendGameController = 1
	gamepadBackendHID            = 2
)

type gamepadDevice struct {
	backend int
	token   uint64
}

func NewGamepad() (*Gamepad, error) {
	if result := int(C.cp_gamepad_initialize()); result != 0 {
		return nil, fmt.Errorf("initialize macOS game controllers: native result %d", result)
	}
	return &Gamepad{known: make(map[core.DeviceID]gamepadDevice)}, nil
}

func (g *Gamepad) Devices() ([]core.DeviceID, error) {
	count := int(C.cp_gamepad_count())
	devices := make([]core.DeviceID, 0, count)
	known := make(map[core.DeviceID]gamepadDevice, count)
	backendIndexes := map[int]int{
		gamepadBackendGameController: 0,
		gamepadBackendHID:            0,
	}
	for index := 0; index < count; index++ {
		var backend C.int
		var token C.uint64_t
		result := int(C.cp_gamepad_device_at(C.int(index), &backend, &token))
		if result < 0 {
			return nil, fmt.Errorf("enumerate macOS game controller %d", index)
		}
		if result == 0 {
			continue
		}
		kind := ""
		switch int(backend) {
		case gamepadBackendGameController:
			kind = "gc"
		case gamepadBackendHID:
			kind = "hid"
		default:
			continue
		}
		device := core.DeviceID(fmt.Sprintf("gamecontroller:%s:%d", kind, backendIndexes[int(backend)]))
		backendIndexes[int(backend)]++
		devices = append(devices, device)
		known[device] = gamepadDevice{backend: int(backend), token: uint64(token)}
	}
	g.mu.Lock()
	g.known = known
	g.mu.Unlock()
	return devices, nil
}

func (g *Gamepad) Read(device core.DeviceID, deadzone float64) (core.State, bool, error) {
	nativeDevice, found := g.nativeDevice(device)
	if !found {
		return core.State{}, false, fmt.Errorf("macOS gamepad %q was not enumerated", device)
	}
	var raw C.cp_gamepad_state
	result := int(C.cp_gamepad_read(C.int(nativeDevice.backend), C.uint64_t(nativeDevice.token), &raw))
	if result == 0 {
		return core.State{}, false, nil
	}
	if result < 0 {
		return core.State{}, false, fmt.Errorf("read macOS game controller %s failed", device)
	}
	leftX, leftY := normalizeStick(float64(raw.left_x), float64(raw.left_y), deadzone)
	rightX, rightY := normalizeStick(float64(raw.right_x), float64(raw.right_y), deadzone)
	return core.State{
		PacketNumber: g.packet.Add(1),
		Buttons:      core.Button(raw.buttons),
		LeftTrigger:  float64(raw.left_trigger), RightTrigger: float64(raw.right_trigger),
		LeftX: leftX, LeftY: leftY, RightX: rightX, RightY: rightY,
	}, true, nil
}

// Apple's GameController API exposes advanced haptics rather than XInput-style
// motor speeds. Do not report a successful pulse that was never delivered.
func (g *Gamepad) Rumble(core.DeviceID, uint16, uint16) error {
	return core.ErrHapticsUnsupported
}

func (g *Gamepad) HapticsSupported(core.DeviceID) bool { return false }

func (g *Gamepad) Diagnostic(device core.DeviceID) (string, error) {
	nativeDevice, found := g.nativeDevice(device)
	if !found {
		return "", fmt.Errorf("macOS gamepad %q was not enumerated", device)
	}
	buffer := C.malloc(4096)
	if buffer == nil {
		return "", fmt.Errorf("allocate diagnostic buffer")
	}
	defer C.free(buffer)
	if C.cp_gamepad_diagnostic(C.int(nativeDevice.backend), C.uint64_t(nativeDevice.token), (*C.char)(buffer), 4096) < 0 {
		return "", fmt.Errorf("read raw macOS game controller %s", device)
	}
	return C.GoString((*C.char)(buffer)), nil
}

func (g *Gamepad) nativeDevice(device core.DeviceID) (gamepadDevice, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nativeDevice, found := g.known[device]
	return nativeDevice, found
}

func normalizeStick(x, y, deadzone float64) (float64, float64) {
	magnitude := math.Min(1, math.Hypot(x, y))
	if magnitude <= deadzone {
		return 0, 0
	}
	scaled := (magnitude - deadzone) / (1 - deadzone)
	return x / magnitude * scaled, y / magnitude * scaled
}
