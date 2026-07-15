//go:build !windows

package platform

import (
	"fmt"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func New(string) (core.Gamepad, core.Desktop, error) {
	return nil, nil, fmt.Errorf("this platform adapter has not been implemented yet")
}
