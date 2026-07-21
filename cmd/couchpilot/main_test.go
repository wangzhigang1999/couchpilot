package main

import (
	"context"
	"errors"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/tray"
)

type fakeApplication struct {
	run func(context.Context) error
}

func (f fakeApplication) Run(ctx context.Context) error { return f.run(ctx) }
func (fakeApplication) Close() error                    { return nil }

var _ tray.Application = fakeApplication{}

func TestRunApplicationStopsUIWhenWorkerFinishes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	workerFailure := errors.New("worker stopped")
	application := fakeApplication{run: func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}}
	workerErr, applicationErr := runApplication(ctx, cancel, application, func(context.Context) error {
		return workerFailure
	})
	if !errors.Is(workerErr, workerFailure) || applicationErr != nil {
		t.Fatalf("worker=%v application=%v", workerErr, applicationErr)
	}
}

func TestRunApplicationStopsWorkerWhenUIFinishes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	applicationFailure := errors.New("application stopped")
	application := fakeApplication{run: func(context.Context) error {
		return applicationFailure
	}}
	workerErr, applicationErr := runApplication(ctx, cancel, application, func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	if workerErr != nil || !errors.Is(applicationErr, applicationFailure) {
		t.Fatalf("worker=%v application=%v", workerErr, applicationErr)
	}
}
