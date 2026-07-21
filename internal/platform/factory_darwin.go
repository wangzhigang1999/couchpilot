//go:build darwin

package platform

import (
	"github.com/wangzhigang1999/couchpilot/internal/core"
	macplatform "github.com/wangzhigang1999/couchpilot/internal/platform/macos"
)

func NewGamepad() (core.Gamepad, error) {
	return macplatform.NewGamepad()
}

func NewDesktop(voiceKey string, appProfiles []core.AppProfile) (core.Desktop, error) {
	return macplatform.NewDesktop(voiceKey, appProfiles)
}
