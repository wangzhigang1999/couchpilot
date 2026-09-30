package engine

import (
	"strings"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

// dispatchAction applies lifecycle semantics regardless of which gesture
// selected the action. Voice and mouse actions cannot bypass their releases.
func (e *Engine) dispatchAction(button core.Button, gesture string, action core.Action) (string, error) {
	if action == core.Voice {
		return string(core.VoiceTap), e.voicePressed(button)
	}
	if down, up, held := heldActionPair(action); held {
		if err := e.desktop.PerformOperation(down); err != nil {
			return "", err
		}
		e.held.mouse[button] = up
		return string(action), nil
	}
	var operation core.Operation
	if strings.HasPrefix(gesture, "lt+") {
		switch action {
		case core.WindowPrevious:
			operation = core.WindowCyclePrevious
		case core.WindowNext:
			operation = core.WindowCycleNext
		}
	}
	if operation != "" {
		e.held.windowSwitching = true
		return string(operation), e.desktop.PerformOperation(operation)
	}
	return string(action), e.desktop.Perform(action)
}
