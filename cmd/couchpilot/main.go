package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/daemon"
	"github.com/wangzhigang1999/couchpilot/internal/engine"
	"github.com/wangzhigang1999/couchpilot/internal/platform"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
	"github.com/wangzhigang1999/couchpilot/internal/tray"
)

const helpText = `CouchPilot

Usage:
  couchpilot [run] [--config config.json] [--verbose]
  couchpilot start [--config config.json] [--verbose]
  couchpilot stop [--config config.json]
  couchpilot status [--config config.json]
  couchpilot install [--config config.json] [--verbose]
  couchpilot uninstall [--config config.json]
  couchpilot doctor [--config config.json]
  couchpilot inspect [--config config.json]
  couchpilot profile [--config config.json]
  couchpilot actions
`

type options struct {
	configPath string
	pidFile    string
	stopFile   string
	verbose    bool
	appLaunch  bool
}

func main() {
	if err := execute(prepareLaunchArgs(os.Args[1:])); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		fmt.Print(helpText)
		return nil
	}
	command := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
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
		if options.pidFile == "" {
			options.pidFile = paths.PIDFile
		}
		if options.stopFile == "" {
			options.stopFile = paths.StopFile
		}
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
	case "install":
		settings, err := config.Load(options.configPath)
		if err != nil {
			return err
		}
		if _, err := newReadyDesktop(settings); err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if err := daemon.InstallAutostart(executable, absoluteConfig, options.verbose); err != nil {
			return err
		}
		fmt.Println("installed startup task and started CouchPilot")
		return nil
	case "uninstall":
		if _, err := daemon.Stop(paths); err != nil {
			return err
		}
		if err := daemon.UninstallAutostart(); err != nil {
			return err
		}
		fmt.Println("removed startup task")
		return nil
	case "doctor":
		return doctor(options.configPath)
	case "inspect":
		return inspect(options.configPath)
	case "profile":
		return showProfile(options.configPath)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, helpText)
	}
}

func inspect(configPath string) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	gamepad, err := platform.NewGamepad()
	if err != nil {
		return err
	}
	devices, err := gamepad.Devices()
	if err != nil {
		return err
	}
	device, found := chooseDevice(settings, devices)
	if !found {
		return errors.New("configured controller is not connected")
	}
	diagnostics, ok := gamepad.(core.GamepadDiagnostics)
	if !ok {
		return errors.New("raw controller diagnostics are not available on this platform")
	}
	fmt.Printf("inspecting %s for 20 seconds; press A, B, X, Y, LB, RB, Back, Start and D-pad in order\n", device)
	deadline := time.Now().Add(20 * time.Second)
	previous := ""
	for time.Now().Before(deadline) {
		raw, err := diagnostics.Diagnostic(device)
		if err != nil {
			return err
		}
		state, connected, err := gamepad.Read(device, settings.Deadzone)
		if err != nil {
			return err
		}
		if !connected {
			return errors.New("controller disconnected during inspect")
		}
		snapshot := fmt.Sprintf("raw=[%s] mapped=0x%04X LT=%.2f RT=%.2f LX=%.2f LY=%.2f RX=%.2f RY=%.2f",
			raw, uint16(state.Buttons), state.LeftTrigger, state.RightTrigger,
			state.LeftX, state.LeftY, state.RightX, state.RightY)
		if snapshot != previous {
			fmt.Println(snapshot)
			previous = snapshot
		}
		time.Sleep(40 * time.Millisecond)
	}
	return nil
}

func parseOptions(command string, args []string) (options, error) {
	set := flag.NewFlagSet(command, flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	var result options
	set.StringVar(&result.configPath, "config", "config.json", "configuration file")
	set.StringVar(&result.pidFile, "pid-file", "", "internal background process pid file")
	set.StringVar(&result.stopFile, "stop-file", "", "internal background process stop file")
	set.BoolVar(&result.verbose, "verbose", false, "log input actions")
	set.BoolVar(&result.appLaunch, "app-launch", false, "internal application-bundle launch mode")
	if err := set.Parse(args); err != nil {
		return options{}, err
	}
	if set.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %v", set.Args())
	}
	return result, nil
}

func run(options options) error {
	if err := redirectAppOutput(options.configPath, options.appLaunch); err != nil {
		return err
	}
	settings, err := config.Load(options.configPath)
	if err != nil {
		return err
	}
	pidClaim, err := daemon.ReservePID(options.pidFile)
	if err != nil {
		return err
	}
	defer pidClaim.Release()
	gamepad, err := platform.NewGamepad()
	if err != nil {
		return err
	}
	desktop, err := newReadyDesktop(settings)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if options.stopFile != "" {
		_ = os.Remove(options.stopFile)
		stopRequest := daemon.StopRequestPath(options.stopFile, os.Getpid())
		_ = os.Remove(stopRequest)
		defer os.Remove(stopRequest)
		go watchStopFile(ctx, stopRequest, cancel)
	}
	paths := daemon.RuntimePaths(options.configPath)
	controller := engine.New(settings, gamepad, desktop, options.verbose, os.Stdout)
	var traceRecorder *trace.Recorder
	if settings.LocalTraceEnabled {
		traceRecorder, err = trace.Open(trace.Options{
			Directory: paths.TraceDirectory,
			OnError: func(err error) {
				fmt.Fprintln(os.Stderr, "local trace:", err)
			},
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "local trace disabled:", err)
			traceRecorder = nil
		} else {
			controller.SetTraceSink(traceRecorder)
			fmt.Printf("local trace: %s\n", trace.Path(paths.TraceDirectory))
		}
	}
	if traceRecorder != nil {
		defer func() {
			if err := traceRecorder.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "close local trace:", err)
			}
		}()
	}
	application, err := tray.New(cancel, tray.Options{
		LogPath:    paths.LogFile,
		ConfigPath: options.configPath,
	})
	if err != nil {
		return fmt.Errorf("create system tray: %w", err)
	}
	defer application.Close()
	if err := pidClaim.MarkReady(); err != nil {
		return fmt.Errorf("publish runtime readiness: %w", err)
	}
	runErr, trayErr := runApplication(ctx, cancel, application, controller.Run)
	if errors.Is(runErr, engine.ErrExitRequested) {
		fmt.Println("emergency exit")
		runErr = nil
	}
	if runErr != nil {
		return runErr
	}
	if trayErr != nil {
		return fmt.Errorf("system tray: %w", trayErr)
	}
	return nil
}

func runApplication(
	ctx context.Context,
	cancel context.CancelFunc,
	application tray.Application,
	worker func(context.Context) error,
) (workerErr, applicationErr error) {
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker(ctx)
		cancel()
	}()
	applicationErr = application.Run(ctx)
	cancel()
	workerErr = <-workerDone
	return workerErr, applicationErr
}

func showProfile(configPath string) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	desktop, err := platform.NewDesktop(settings.VoiceKey, settings.AppProfiles)
	if err != nil {
		return err
	}
	profile, _ := desktop.ForegroundContext()
	fmt.Println(profile)
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
	gamepad, err := platform.NewGamepad()
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
	hapticsSupported := true
	if capabilities, ok := gamepad.(core.GamepadCapabilities); ok {
		hapticsSupported = capabilities.HapticsSupported(device)
	}
	if hapticsSupported {
		if err := gamepad.Rumble(device, 36000, 26000); err != nil {
			return err
		}
		time.Sleep(180 * time.Millisecond)
		_ = gamepad.Rumble(device, 0, 0)
		fmt.Println("controller haptics: test pulse sent")
	} else {
		fmt.Println("controller haptics: unsupported (test skipped)")
	}
	if _, err := newReadyDesktop(settings); err != nil {
		return fmt.Errorf("desktop automation: %w", err)
	}
	fmt.Println("desktop automation: ready")
	return nil
}

func newReadyDesktop(settings config.Settings) (core.Desktop, error) {
	desktop, err := platform.NewDesktop(settings.VoiceKey, settings.AppProfiles)
	if err != nil {
		return nil, err
	}
	if readiness, ok := desktop.(core.Readiness); ok {
		if err := readiness.Ready(); err != nil {
			return nil, fmt.Errorf("desktop adapter is not ready: %w", err)
		}
	}
	return desktop, nil
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
		if settings.ControllerIndex < len(devices) {
			return devices[settings.ControllerIndex], true
		}
		return "", false
	}
	if len(devices) == 0 {
		return "", false
	}
	return devices[0], true
}
