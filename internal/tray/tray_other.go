//go:build !windows

package tray

import "context"

// Start is a no-op outside Windows. It still mirrors the Windows lifetime so
// the worker can wait for tray cleanup without platform-specific branching.
func Start(ctx context.Context, _ context.CancelFunc, _, _ string) (<-chan error, error) {
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		done <- nil
		close(done)
	}()
	return done, nil
}
