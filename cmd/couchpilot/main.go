package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/daemon"
	"github.com/wangzhigang1999/couchpilot/internal/engine"
	"github.com/wangzhigang1999/couchpilot/internal/platform"
)

const usage = `CouchPilot

Usage:
  couchpilot [run] [--config config.json] [--verbose]
  couchpilot start [--config config.json] [--verbose]
  couchpilot stop [--config config.json]
  couchpilot status [--config config.json]
  couchpilot doctor [--config config.json]
  couchpilot profile [--config config.json]
  couchpilot actions
`

type options struct {
	configPath string
	pidFile    string
	stopFile   string
	verbose    bool
}

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	command := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if command == "help" || command == "--help" || command == "-h" {
		fmt.Print(usage)
		return nil
	}
	if command == "actions" {
		for _, action := range core.KnownActions {
			fmt.Println(action)
		}
		return nil
	}
	options, err := parseOptions(command, args)
	if err != nil {
		return err
	}
	absoluteConfig, err := filepath.Abs(options.configPath)
	if err != nil {
		return err
	}
	options.configPath = absoluteConfig
	paths := daemon.RuntimePaths(absoluteConfig)
	switch command {
	case "run":
		return run(options)
	case "start":
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		pid, err := daemon.Start(executable, absoluteConfig, options.verbose)
		if err != nil {
			return err
		}
		fmt.Printf("started pid %d\nlog: %s\n", pid, paths.LogFile)
		return nil
	case "stop":
		pid, err := daemon.Stop(paths)
		if err != nil {
			return err
		}
		if pid == 0 {
			fmt.Println("not running")
		} else {
			fmt.Printf("stopped pid %d\n", pid)
		}
		return nil
	case "status":
		if pid, running := daemon.Status(paths.PIDFile); running {
			fmt.Printf("running pid %d\n", pid)
		} else {
			fmt.Println("not running")
		}
		return nil
	case "doctor":
		return doctor(options.configPath)
	case "profile":
		return showProfile(options.configPath)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, usage)
	}
}

func parseOptions(command string, args []string) (options, error) {
	set := flag.NewFlagSet(command, flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	var result options
	set.StringVar(&result.configPath, "config", "config.json", "configuration file")
	set.StringVar(&result.pidFile, "pid-file", "", "internal background process pid file")
	set.StringVar(&result.stopFile, "stop-file", "", "internal background process stop file")
	set.BoolVar(&result.verbose, "verbose", false, "log input actions")
	if err := set.Parse(args); err != nil {
		return options{}, err
	}
	if set.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %v", set.Args())
	}
	return result, nil
}

func run(options options) error {
	settings, err := config.Load(options.configPath)
	if err != nil {
		return err
	}
	releasePID, err := daemon.ClaimPID(options.pidFile)
	if err != nil {
		return err
	}
	defer releasePID()
	gamepad, desktop, err := platform.New(settings.VoiceKey, settings.AppProfiles)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if options.stopFile != "" {
		defer os.Remove(options.stopFile)
		go watchStopFile(ctx, options.stopFile, cancel)
	}
	controller := engine.New(settings, gamepad, desktop, options.verbose, os.Stdout)
	err = controller.Run(ctx)
	if errors.Is(err, engine.ErrExitRequested) {
		fmt.Println("emergency exit")
		return nil
	}
	return err
}

func showProfile(configPath string) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	_, desktop, err := platform.New(settings.VoiceKey, settings.AppProfiles)
	if err != nil {
		return err
	}
	fmt.Println(desktop.ForegroundProfile())
	return nil
}

func watchStopFile(ctx context.Context, path string, cancel context.CancelFunc) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				cancel()
				return
			}
		}
	}
}

func doctor(configPath string) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	gamepad, _, err := platform.New(settings.VoiceKey, settings.AppProfiles)
	if err != nil {
		return err
	}
	devices, err := gamepad.Devices()
	if err != nil {
		return err
	}
	fmt.Printf("controllers: %v\n", devices)
	device, found := chooseDevice(settings, devices)
	if !found {
		return errors.New("configured controller is not connected")
	}
	state, connected, err := gamepad.Read(device, settings.Deadzone)
	if err != nil {
		return err
	}
	if !connected {
		return errors.New("controller disconnected during doctor")
	}
	fmt.Printf("%s ready; packet=%d buttons=0x%04X LT=%.2f RT=%.2f\n",
		device, state.PacketNumber, uint16(state.Buttons), state.LeftTrigger, state.RightTrigger)
	if err := gamepad.Rumble(device, 36000, 26000); err != nil {
		return err
	}
	time.Sleep(180 * time.Millisecond)
	_ = gamepad.Rumble(device, 0, 0)
	fmt.Println("vibration test sent")
	return nil
}

func chooseDevice(settings config.Settings, devices []core.DeviceID) (core.DeviceID, bool) {
	if settings.DeviceID != "" {
		for _, device := range devices {
			if string(device) == settings.DeviceID {
				return device, true
			}
		}
		return "", false
	}
	if settings.ControllerIndex >= 0 {
		suffix := ":" + strconv.Itoa(settings.ControllerIndex)
		for _, device := range devices {
			if strings.HasSuffix(string(device), suffix) {
				return device, true
			}
		}
		return "", false
	}
	if len(devices) == 0 {
		return "", false
	}
	return devices[0], true
}
