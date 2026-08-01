//go:build darwin

package platform

import (
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/desktop"
	macplatform "github.com/wangzhigang1999/couchpilot/internal/platform/macos"
)

func NewGamepad() (core.Gamepad, error) {
	return macplatform.NewGamepad()
}

func NewDesktop(voiceKey string, appProfiles []core.AppProfile) (core.Desktop, error) {
	driver, err := macplatform.NewDesktop(voiceKey)
	if err != nil {
		return nil, err
	}
	return desktop.NewSmoothExecutor(driver, appProfiles), nil
}
