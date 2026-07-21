//go:build windows

package platform

import (
	"github.com/wangzhigang1999/couchpilot/internal/core"
	winplatform "github.com/wangzhigang1999/couchpilot/internal/platform/windows"
)

func NewGamepad() (core.Gamepad, error) {
	return winplatform.NewGamepad()
}

func NewDesktop(voiceKey string, appProfiles []core.AppProfile) (core.Desktop, error) {
	return winplatform.NewDesktop(voiceKey, appProfiles)
}
