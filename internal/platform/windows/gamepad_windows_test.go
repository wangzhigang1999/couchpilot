package winplatform

import "testing"

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
