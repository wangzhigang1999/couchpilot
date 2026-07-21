package tray

import "context"

// Options is the platform-independent configuration for the system tray.
// Keeping paths in one value prevents build-tag implementations from drifting
// when a new menu action is added.
type Options struct {
	LogPath    string
	ConfigPath string
}

// Application owns the native UI loop. Run must be called from the process's
// main goroutine; platform implementations pin that goroutine to an OS thread
// where required.
type Application interface {
	Run(context.Context) error
	Close() error
}
