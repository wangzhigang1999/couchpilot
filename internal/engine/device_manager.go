package engine

import (
	"math"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func (e *Engine) findDevice() (core.DeviceID, bool, error) {
	devices, err := e.gamepad.Devices()
	if err != nil {
		return "", false, err
	}
	if e.options.DeviceID != "" {
		wanted := core.DeviceID(e.options.DeviceID)
		for _, device := range devices {
			if device == wanted {
				return device, true, nil
			}
		}
		return "", false, nil
	}
	if e.options.ControllerIndex >= 0 {
		if e.options.ControllerIndex < len(devices) {
			return devices[e.options.ControllerIndex], true, nil
		}
		return "", false, nil
	}
	if len(devices) == 0 {
		return "", false, nil
	}
	return devices[0], true, nil
}

// findAlternativeInput lets any controller take over when device selection is
// automatic. The current controller keeps priority while it is actively used.
func (e *Engine) findAlternativeInput(current core.State) (core.DeviceID, core.State, bool, error) {
	if e.options.DeviceID != "" || e.options.ControllerIndex >= 0 || stateHasInput(current) {
		return "", core.State{}, false, nil
	}
	devices, err := e.gamepad.Devices()
	if err != nil {
		return "", core.State{}, false, err
	}
	for _, device := range devices {
		if device == e.device {
			continue
		}
		state, connected, err := e.gamepad.Read(device, e.options.Deadzone)
		if err != nil {
			return "", core.State{}, false, err
		}
		if connected && stateHasInput(state) {
			return device, state, true, nil
		}
	}
	return "", core.State{}, false, nil
}

func stateHasInput(state core.State) bool {
	return state.Buttons != 0 ||
		state.LeftTrigger > 0.08 || state.RightTrigger > 0.08 ||
		math.Abs(state.LeftX) > 1e-4 || math.Abs(state.LeftY) > 1e-4 ||
		math.Abs(state.RightX) > 1e-4 || math.Abs(state.RightY) > 1e-4
}
