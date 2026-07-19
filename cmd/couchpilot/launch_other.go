//go:build !windows

package main

func prepareLaunchArgs(args []string) []string {
	return args
}
