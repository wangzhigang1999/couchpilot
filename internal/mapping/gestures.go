package mapping

import (
	"strings"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

type ButtonGesture struct {
	Button  core.Button
	Gesture string
}

// ButtonGestures is the shared vocabulary for configuration and input dispatch.
// Return a value so callers cannot mutate the catalog. Back/Start are reserved.
func ButtonGestures() [12]ButtonGesture {
	return [12]ButtonGesture{
		{core.DPadUp, "dpad_up"}, {core.DPadDown, "dpad_down"},
		{core.DPadLeft, "dpad_left"}, {core.DPadRight, "dpad_right"},
		{core.LeftShoulder, "lb"}, {core.RightShoulder, "rb"},
		{core.LeftThumb, "l3"}, {core.RightThumb, "r3"},
		{core.A, "a"}, {core.B, "b"}, {core.X, "x"}, {core.Y, "y"},
	}
}

func IsSupportedGesture(gesture string) bool {
	if gesture == "voice+a" || gesture == "voice+b" {
		return true
	}
	base := gesture
	if modifier, suffix, found := strings.Cut(gesture, "+"); found {
		if modifier != "lt" && modifier != "rt" {
			return false
		}
		base = suffix
	}
	for _, item := range ButtonGestures() {
		if item.Gesture == base {
			return true
		}
	}
	return false
}
