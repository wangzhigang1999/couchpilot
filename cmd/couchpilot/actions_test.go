package main

import (
	"os"
	"strings"
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestExecuteActionsArguments(t *testing.T) {
	var actionList strings.Builder
	for _, action := range core.KnownActions {
		actionList.WriteString(string(action) + "\n")
	}
	for _, test := range []struct {
		name     string
		args     []string
		wantErr  bool
		wantHelp bool
	}{
		{name: "list"},
		{name: "empty terminator", args: []string{"--"}},
		{name: "long help", args: []string{"--help"}, wantHelp: true},
		{name: "short help", args: []string{"-h"}, wantHelp: true},
		{name: "unknown flag", args: []string{"--unknown-option"}, wantErr: true},
		{name: "positional argument", args: []string{"unexpected"}, wantErr: true},
		{name: "argument after terminator", args: []string{"--", "unexpected"}, wantErr: true},
		{name: "unused config flag", args: []string{"--config", "config.json"}, wantErr: true},
		{name: "unused verbose flag", args: []string{"--verbose"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// execute writes to process streams, so these cases must stay serial.
			stdout, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { stdout.Close() })
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { stderr.Close() })
			previousOut, previousErr := os.Stdout, os.Stderr
			os.Stdout, os.Stderr = stdout, stderr
			t.Cleanup(func() { os.Stdout, os.Stderr = previousOut, previousErr })

			err = execute(append([]string{"actions"}, test.args...))
			if (err != nil) != test.wantErr {
				t.Errorf("execute error = %v, want error = %v", err, test.wantErr)
			}
			out, err := os.ReadFile(stdout.Name())
			if err != nil {
				t.Fatal(err)
			}
			diagnostics, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if test.wantErr || test.wantHelp {
				if len(out) != 0 {
					t.Errorf("help or invalid arguments printed actions: %s", out)
				}
			} else if string(out) != actionList.String() || len(diagnostics) != 0 {
				t.Errorf("unexpected action list output: stdout=%q stderr=%q", out, diagnostics)
			}
			if test.wantHelp && (!strings.Contains(string(diagnostics), "Usage of actions:") || strings.Contains(string(diagnostics), "-config")) {
				t.Errorf("expected actions-only help, got %q", diagnostics)
			}
		})
	}
}
