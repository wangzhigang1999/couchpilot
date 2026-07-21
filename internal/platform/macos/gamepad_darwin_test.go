//go:build darwin

package macplatform

import (
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestNormalizeStickDeadzoneAndRange(t *testing.T) {
	x, y := normalizeStick(0.05, 0.05, 0.18)
	if x != 0 || y != 0 {
		t.Fatalf("expected deadzone, got %f %f", x, y)
	}
	x, y = normalizeStick(1, 0, 0.18)
	if x != 1 || y != 0 {
		t.Fatalf("expected full right, got %f %f", x, y)
	}
}

func TestNativeDeviceLookupUsesStablePublicID(t *testing.T) {
	gamepad := &Gamepad{known: map[core.DeviceID]gamepadDevice{
		"gamecontroller:hid:0": {backend: gamepadBackendHID, token: 0xa2},
	}}
	device, found := gamepad.nativeDevice("gamecontroller:hid:0")
	if !found || device.backend != gamepadBackendHID || device.token != 0xa2 {
		t.Fatalf("device=%+v found=%t", device, found)
	}
	if _, found := gamepad.nativeDevice("gamecontroller:hid:1"); found {
		t.Fatal("unexpected unknown device")
	}
}
