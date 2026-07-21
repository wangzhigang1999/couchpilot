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
	"github.com/wangzhigang1999/couchpilot/internal/tray"
	usagepkg "github.com/wangzhigang1999/couchpilot/internal/usage"
)

const usage = `CouchPilot

Usage:
  couchpilot [run] [--config config.json] [--verbose]
  couchpilot start [--config config.json] [--verbose]
  couchpilot stop [--config config.json]
  couchpilot status [--config config.json]
  couchpilot install [--config config.json] [--verbose]
  couchpilot uninstall [--config config.json]
  couchpilot doctor [--config config.json]
  couchpilot profile [--config config.json]
  couchpilot usage [--config config.json]
  couchpilot actions
`

type options struct {
	configPath string
	pidFile    string
	stopFile   string
	verbose    bool
}

func main() {
	if err := execute(prepareLaunchArgs(os.Args[1:])); err != nil {
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
	case "profile":
		return showProfile(options.configPath)
	case "usage":
		return showUsage(options.configPath)
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
	paths := daemon.RuntimePaths(options.configPath)
	controller := engine.New(settings, gamepad, desktop, options.verbose, os.Stdout)
	var usageRecorder *usagepkg.FileRecorder
	usageReportPath := ""
	if settings.LocalUsageStatsEnabled {
		usageRecorder, err = usagepkg.Open(usagepkg.Options{
			Directory:  paths.UsageDirectory,
			Inventory:  controller.BindingInventory(),
			StrategyID: controller.StrategyRevision(),
			Controls:   controller.UsageControls(),
			OnError: func(err error) {
				fmt.Fprintln(os.Stderr, "local usage stats:", err)
			},
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "local usage stats disabled:", err)
			usageRecorder = nil
		} else {
			controller.SetUsageRecorder(usageRecorder)
			usageReportPath = paths.UsageReportFile
			fmt.Printf("local usage stats: %s\n", paths.UsageSnapshotFile)
		}
	}
	// Keep an existing report reachable even when new observations are disabled
	// (or a recorder could not be opened). Disabling collection never deletes
	// the user's historical local report.
	if usageReportPath == "" {
		if _, statErr := os.Stat(paths.UsageReportFile); statErr == nil {
			usageReportPath = paths.UsageReportFile
		}
	}
	trayDone, err := tray.Start(ctx, cancel, paths.LogFile, options.configPath, usageReportPath)
	if err != nil {
		if usageRecorder != nil {
			_ = usageRecorder.Close()
		}
		return fmt.Errorf("start system tray: %w", err)
	}
	runErr := controller.Run(ctx)
	if usageRecorder != nil {
		if err := usageRecorder.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "close local usage stats:", err)
		}
	}
	cancel()
	trayErr := <-trayDone
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

func showUsage(configPath string) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	paths := daemon.RuntimePaths(configPath)
	state := "已开启"
	if !settings.LocalUsageStatsEnabled {
		state = "已关闭"
	}
	fmt.Printf("本地键位策略记录：%s\n", state)
	summary, err := usagepkg.ReadSummary(paths.UsageDirectory)
	if errors.Is(err, usagepkg.ErrNoUsageData) {
		fmt.Println("尚无按键记录。启动 CouchPilot 并使用手柄后再查看。")
		fmt.Printf("记录目录：%s\n", paths.UsageDirectory)
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取本地按键报告: %w", err)
	}
	fmt.Println()
	fmt.Println("用途：用真实的按键、组合、App 场景和粗粒度 tracing 优化键位策略。")
	fmt.Println("口径：“派发成功/派发失败”只表示系统动作是否成功发出，不代表用户意图是否达成。")
	fmt.Println()
	fmt.Print(usagepkg.FormatText(summary))
	fmt.Printf("\nHTML 报告：%s\n", paths.UsageReportFile)
	fmt.Printf("聚合快照：%s\n", paths.UsageSnapshotFile)
	fmt.Printf("崩溃恢复日志：%s\n", paths.UsageWALFile)
	return nil
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
