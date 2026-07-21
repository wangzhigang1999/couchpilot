package winplatform

import (
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

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
