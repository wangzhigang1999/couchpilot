package winplatform

import (
	"fmt"
	"math"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestNormalizeStickClampsLengthWithoutChangingDirection(t *testing.T) {
	for _, input := range []struct {
		name string
		x, y int16
	}{
		{"upper right", 32767, 32767},
		{"upper left", -32768, 32767},
		{"lower left", -32768, -32768},
		{"lower right", 32767, -32768},
		{"unequal axes", 32767, 16384},
	} {
		for _, deadzone := range []float64{0, 0.18, 0.9} {
			t.Run(fmt.Sprintf("%s/deadzone=%.2f", input.name, deadzone), func(t *testing.T) {
				x, y := normalizeStick(input.x, input.y, deadzone)
				if length := math.Hypot(x, y); math.Abs(length-1) > 1e-12 {
					t.Errorf("output length=%f, want 1 (x=%f y=%f)", length, x, y)
				}
				rawX := math.Max(-1, float64(input.x)/32767)
				rawY := math.Max(-1, float64(input.y)/32767)
				if math.Abs(x*rawY-y*rawX) > 1e-12 || x*rawX+y*rawY <= 0 {
					t.Errorf("direction changed: raw=(%f,%f), normalized=(%f,%f)", rawX, rawY, x, y)
				}
			})
		}
	}
}

func TestNormalizeStickDeadzoneAndRange(t *testing.T) {
	x, y := normalizeStick(100, 100, 0.18)
	if x != 0 || y != 0 {
		t.Fatalf("expected deadzone, got %f %f", x, y)
	}
	x, y = normalizeStick(32767, 0, 0.18)
	if x != 1 || y != 0 {
		t.Fatalf("expected full right, got %f %f", x, y)
	}
}

func TestXInputTriggersAreUnsigned(t *testing.T) {
	raw := xinputGamepad{LeftTrigger: 255, RightTrigger: 200}
	if raw.LeftTrigger != 255 || raw.RightTrigger != 200 {
		t.Fatalf("trigger values changed: %+v", raw)
	}
}

func TestMapXInputButtonsUsesPortableButtonLayout(t *testing.T) {
	raw := uint16(0x0001 | 0x0020 | 0x0200 | 0x1000 | 0x2000 | 0x4000 | 0x8000)
	want := core.DPadUp | core.Back | core.RightShoulder | core.A | core.B | core.X | core.Y
	if got := mapXInputButtons(raw); got != want {
		t.Fatalf("buttons=0x%04X want 0x%04X", uint16(got), uint16(want))
	}
}
