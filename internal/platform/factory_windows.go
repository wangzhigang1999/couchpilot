//go:build windows

package platform

import (
	"github.com/wangzhigang1999/couchpilot/internal/core"
	winplatform "github.com/wangzhigang1999/couchpilot/internal/platform/windows"
)

func New(voiceKey string, appProfiles []core.AppProfile) (core.Gamepad, core.Desktop, error) {
	gamepad, err := winplatform.NewGamepad()
	if err != nil {
		return nil, nil, err
	}
	desktop, err := winplatform.NewDesktop(voiceKey, appProfiles)
	if err != nil {
		return nil, nil, err
	}
	return gamepad, desktop, nil
}
