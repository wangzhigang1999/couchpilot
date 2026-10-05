package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/tray"
)

func TestExecuteSubcommandHelpSucceedsWithoutCreatingRuntimeFiles(t *testing.T) {
	for _, command := range []string{"run", "start", "stop", "status", "doctor", "inspect", "profile"} {
		for _, help := range []string{"--help", "-h"} {
			t.Run(command+"/"+help, func(t *testing.T) {
				directory := t.TempDir()
				configPath := filepath.Join(directory, "config.json")
				if err := execute([]string{command, "--config", configPath, help}); err != nil {
					t.Fatalf("help returned an error: %v", err)
				}
				entries, err := os.ReadDir(directory)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Fatalf("help created runtime files: %v", entries)
				}
			})
		}
	}
}

func TestExecuteStillRejectsInvalidOptions(t *testing.T) {
	for _, args := range [][]string{
		{"status", "--unknown-option"},
		{"status", "--config"},
		{"status", "--verbose=invalid"},
		{"status", "unexpected-argument"},
	} {
		t.Run(args[1], func(t *testing.T) {
			if err := execute(args); err == nil {
				t.Fatal("invalid options were accepted")
			}
		})
	}
}

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
