//go:build !windows && !darwin

package main

func prepareLaunchArgs(args []string) []string {
	return args
}

func redirectAppOutput(string, bool) error { return nil }
