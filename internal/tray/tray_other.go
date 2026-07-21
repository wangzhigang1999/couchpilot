//go:build !windows && !darwin

package tray

import "context"

type noopApplication struct{}

func New(context.CancelFunc, Options) (Application, error) {
	return noopApplication{}, nil
}

func (noopApplication) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (noopApplication) Close() error { return nil }
