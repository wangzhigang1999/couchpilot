//go:build !windows && !darwin

package platform

import (
	"fmt"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func NewGamepad() (core.Gamepad, error) {
	return nil, fmt.Errorf("this platform gamepad adapter has not been implemented yet")
}

func NewDesktop(string, []core.AppProfile) (core.Desktop, error) {
	return nil, fmt.Errorf("this platform desktop adapter has not been implemented yet")
}
