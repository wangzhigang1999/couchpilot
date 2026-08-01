//go:build windows

package platform

import (
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/desktop"
	winplatform "github.com/wangzhigang1999/couchpilot/internal/platform/windows"
)

func NewGamepad() (core.Gamepad, error) {
	return winplatform.NewGamepad()
}

func NewDesktop(voiceKey string, appProfiles []core.AppProfile) (core.Desktop, error) {
	driver, err := winplatform.NewDesktop(voiceKey)
	if err != nil {
		return nil, err
	}
	return desktop.NewExecutor(driver, appProfiles), nil
}
