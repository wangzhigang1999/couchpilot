//go:build darwin

package macplatform

import (
	"fmt"
	"math"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestNormalizeStickClampsLengthWithoutChangingDirection(t *testing.T) {
	for _, input := range []struct {
		name string
		x, y float64
	}{
		{"upper right", 1, 1},
		{"upper left", -1, 1},
		{"lower left", -1, -1},
		{"lower right", 1, -1},
		{"unequal axes", 1, 0.5},
	} {
		for _, deadzone := range []float64{0, 0.18, 0.9} {
			t.Run(fmt.Sprintf("%s/deadzone=%.2f", input.name, deadzone), func(t *testing.T) {
				x, y := normalizeStick(input.x, input.y, deadzone)
				if length := math.Hypot(x, y); math.Abs(length-1) > 1e-12 {
					t.Errorf("output length=%f, want 1 (x=%f y=%f)", length, x, y)
				}
				if math.Abs(x*input.y-y*input.x) > 1e-12 || x*input.x+y*input.y <= 0 {
					t.Errorf("direction changed: raw=(%f,%f), normalized=(%f,%f)", input.x, input.y, x, y)
				}
			})
		}
	}
}

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
