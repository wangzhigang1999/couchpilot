package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
)

var ErrExitRequested = errors.New("emergency exit requested")

type Engine struct {
	settings config.Settings
	gamepad  core.Gamepad
	desktop  core.Desktop
	clock    core.Clock
	resolver *Resolver
	logger   *log.Logger
	verbose  bool

	device               core.DeviceID
	previousButtons      core.Button
	previousLeftTrigger  bool
	previousRightTrigger bool
	voiceHeld            core.Button
	exitComboStarted     time.Time
	rumbleUntil          time.Time
	rumbleActive         bool
	windowSwitching      bool
	heldActions          map[core.Button]core.Action
	scrollRemainder      float64
	moveRemainder        [2]float64
}

func New(settings config.Settings, gamepad core.Gamepad, desktop core.Desktop, verbose bool, output io.Writer) *Engine {
	if output == nil {
		output = io.Discard
	}
	return &Engine{
		settings:    settings,
		gamepad:     gamepad,
		desktop:     desktop,
		clock:       core.RealClock{},
		resolver:    NewResolver(settings.Bindings),
		logger:      log.New(output, "", log.LstdFlags),
		verbose:     verbose,
		heldActions: make(map[core.Button]core.Action),
	}
}

func (e *Engine) Run(ctx context.Context) error {
	e.logger.Printf("gamepad: waiting for input")
	e.logger.Printf("emergency exit: hold Back + Start for %.1f seconds", e.settings.ExitHoldSeconds)
	last := e.clock.Now()
	defer e.shutdown()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		frameStarted := e.clock.Now()
		dt := frameStarted.Sub(last).Seconds()
		if dt > 0.05 {
			dt = 0.05
		}
		last = frameStarted
		if e.device == "" {
			device, found, err := e.findDevice()
			if err != nil {
				return err
			}
			if !found {
				e.clock.Sleep(500 * time.Millisecond)
				continue
			}
			e.device = device
			e.logger.Printf("using controller %s", device)
		}
		state, connected, err := e.gamepad.Read(e.device, e.settings.Deadzone)
		if err != nil {
			return err
		}
		if !connected {
			e.logger.Printf("controller disconnected; waiting for reconnect")
			e.disconnect()
			e.clock.Sleep(500 * time.Millisecond)
			continue
		}
		if err := e.Step(state, dt, frameStarted); err != nil {
			return err
		}
		wait := time.Second/time.Duration(e.settings.PollHz) - e.clock.Now().Sub(frameStarted)
		if wait > 0 {
			e.clock.Sleep(wait)
		}
	}
}

func (e *Engine) Step(state core.State, dt float64, now time.Time) error {
	leftTriggerWasActive := e.previousLeftTrigger
	e.logEdges(state)
	if leftTriggerWasActive && state.LeftTrigger <= 0.08 && e.windowSwitching {
		if err := e.finishWindowSwitch(); err != nil {
			return err
		}
	}
	combo := core.Back | core.Start
	if state.Buttons&combo == combo {
		if e.exitComboStarted.IsZero() {
			e.exitComboStarted = now
		} else if now.Sub(e.exitComboStarted).Seconds() >= e.settings.ExitHoldSeconds {
			return ErrExitRequested
		}
	} else {
		e.exitComboStarted = time.Time{}
	}
	if err := e.movePointer(state, dt); err != nil {
		return err
	}
	if err := e.scroll(state, dt); err != nil {
		return err
	}
	if err := e.buttons(state); err != nil {
		return err
	}
	e.updateRumble(now)
	e.previousButtons = state.Buttons
	return nil
}

func (e *Engine) findDevice() (core.DeviceID, bool, error) {
	devices, err := e.gamepad.Devices()
	if err != nil {
		return "", false, err
	}
	if e.settings.DeviceID != "" {
		wanted := core.DeviceID(e.settings.DeviceID)
		for _, device := range devices {
			if device == wanted {
				return device, true, nil
			}
		}
		return "", false, nil
	}
	if e.settings.ControllerIndex >= 0 {
		suffix := ":" + strconv.Itoa(e.settings.ControllerIndex)
		for _, device := range devices {
			if strings.HasSuffix(string(device), suffix) {
				return device, true, nil
			}
		}
		return "", false, nil
	}
	if len(devices) == 0 {
		return "", false, nil
	}
	return devices[0], true, nil
}

func (e *Engine) movePointer(state core.State, dt float64) error {
	x, y := state.LeftX, -state.LeftY
	magnitude := math.Hypot(x, y)
	if magnitude < 1e-4 {
		e.moveRemainder = [2]float64{}
		return nil
	}
	multiplier := 1.0
	if state.LeftTrigger > 0.08 {
		amount := math.Min(1, (state.LeftTrigger-0.08)/0.92)
		multiplier = 1 - amount*(1-e.settings.PrecisionSpeedMultiplier)
	} else if state.RightTrigger > 0.08 {
		amount := math.Min(1, (state.RightTrigger-0.08)/0.92)
		multiplier = 1 + amount*(e.settings.BoostSpeedMultiplier-1)
	}
	speed := e.settings.PointerMaxSpeed * math.Pow(magnitude, e.settings.PointerCurve) * multiplier
	e.moveRemainder[0] += x / magnitude * speed * dt
	e.moveRemainder[1] += y / magnitude * speed * dt
	dx, dy := int(e.moveRemainder[0]), int(e.moveRemainder[1])
	e.moveRemainder[0] -= float64(dx)
	e.moveRemainder[1] -= float64(dy)
	if dx == 0 && dy == 0 {
		return nil
	}
	return e.desktop.MovePointer(dx, dy)
}

func (e *Engine) scroll(state core.State, dt float64) error {
	e.scrollRemainder += state.RightY * e.settings.ScrollUnitsPerSecond * dt
	for math.Abs(e.scrollRemainder) >= 120 {
		amount := 120
		if e.scrollRemainder < 0 {
			amount = -120
		}
		if err := e.desktop.Scroll(amount); err != nil {
			return err
		}
		e.scrollRemainder -= float64(amount)
	}
	return nil
}

var gestures = []struct {
	button  core.Button
	gesture string
}{
	{core.DPadUp, "dpad_up"}, {core.DPadDown, "dpad_down"},
	{core.DPadLeft, "dpad_left"}, {core.DPadRight, "dpad_right"},
	{core.LeftShoulder, "lb"}, {core.RightShoulder, "rb"},
	{core.LeftThumb, "l3"}, {core.RightThumb, "r3"},
	{core.A, "a"}, {core.B, "b"}, {core.X, "x"}, {core.Y, "y"},
}

func (e *Engine) buttons(state core.State) error {
	pressed := core.Button(uint16(state.Buttons) &^ uint16(e.previousButtons))
	released := core.Button(uint16(e.previousButtons) &^ uint16(state.Buttons))
	for _, item := range gestures {
		if released&item.button == 0 {
			continue
		}
		if err := e.releaseHeldAction(item.button); err != nil {
			return err
		}
	}
	if released&e.voiceHeld != 0 {
		for _, item := range gestures {
			if released&item.button != 0 && e.voiceHeld&item.button != 0 {
				if err := e.voiceReleased(); err != nil {
					return err
				}
				e.voiceHeld &^= item.button
			}
		}
	}
	if pressed == 0 {
		return nil
	}
	profile := e.desktop.ForegroundProfile()
	for _, item := range gestures {
		if pressed&item.button == 0 {
			continue
		}
		gesture := item.gesture
		if state.LeftTrigger > 0.08 {
			if _, found := e.resolver.Resolve(profile, "lt+"+gesture); found {
				gesture = "lt+" + gesture
			}
		} else if state.RightTrigger > 0.08 {
			if _, found := e.resolver.Resolve(profile, "rt+"+gesture); found {
				gesture = "rt+" + gesture
			}
		}
		action, found := e.resolver.Resolve(profile, gesture)
		if !found {
			continue
		}
		if action == core.Voice {
			if err := e.voicePressed(item.button); err != nil {
				return err
			}
			continue
		}
		if down, up, held := heldActionPair(action); held {
			if err := e.desktop.Perform(down); err != nil {
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.heldActions[item.button] = up
			continue
		}
		performedAction := action
		if strings.HasPrefix(gesture, "lt+") {
			switch action {
			case core.WindowPrevious:
				performedAction = core.WindowCyclePrevious
				e.windowSwitching = true
			case core.WindowNext:
				performedAction = core.WindowCycleNext
				e.windowSwitching = true
			}
		}
		if err := e.desktop.Perform(performedAction); err != nil {
			if e.windowSwitching {
				_ = e.finishWindowSwitch()
			}
			return fmt.Errorf("perform %s: %w", action, err)
		}
		if profile != "default" || strings.HasPrefix(gesture, "lt+") || strings.HasPrefix(gesture, "rt+") {
			e.rumbleUntil = e.clock.Now().Add(25 * time.Millisecond)
		}
		if e.verbose {
			e.logger.Printf("%s/%s -> %s", profile, gesture, action)
		}
	}
	return nil
}

func heldActionPair(action core.Action) (core.Action, core.Action, bool) {
	switch action {
	case core.ClickLeft:
		return core.MouseLeftDown, core.MouseLeftUp, true
	case core.ClickRight:
		return core.MouseRightDown, core.MouseRightUp, true
	default:
		return "", "", false
	}
}

func (e *Engine) releaseHeldAction(button core.Button) error {
	action, found := e.heldActions[button]
	if !found {
		return nil
	}
	delete(e.heldActions, button)
	return e.desktop.Perform(action)
}

func (e *Engine) releaseAllHeldActions() {
	for button := range e.heldActions {
		_ = e.releaseHeldAction(button)
	}
}

func (e *Engine) voicePressed(button core.Button) error {
	switch e.settings.VoiceMode {
	case "tap":
		return e.desktop.Perform(core.VoiceTap)
	case "toggle_while_held":
		e.voiceHeld |= button
		return e.desktop.Perform(core.VoiceTap)
	case "hold":
		e.voiceHeld |= button
		return e.desktop.Perform(core.VoiceDown)
	default:
		return nil
	}
}

func (e *Engine) voiceReleased() error {
	if e.settings.VoiceMode == "toggle_while_held" {
		return e.desktop.Perform(core.VoiceTap)
	}
	if e.settings.VoiceMode == "hold" {
		return e.desktop.Perform(core.VoiceUp)
	}
	return nil
}

func (e *Engine) finishWindowSwitch() error {
	if !e.windowSwitching {
		return nil
	}
	e.windowSwitching = false
	return e.desktop.Perform(core.WindowCycleCommit)
}

func (e *Engine) logEdges(state core.State) {
	left := state.LeftTrigger > 0.08
	right := state.RightTrigger > 0.08
	if e.verbose && left != e.previousLeftTrigger {
		e.logger.Printf("LT precision: %t", left)
	}
	if e.verbose && right != e.previousRightTrigger {
		e.logger.Printf("RT boost: %t", right)
	}
	e.previousLeftTrigger = left
	e.previousRightTrigger = right
}

func (e *Engine) updateRumble(now time.Time) {
	active := now.Before(e.rumbleUntil)
	if active == e.rumbleActive || e.device == "" {
		return
	}
	var amount uint16
	if active {
		amount = 9000
	}
	_ = e.gamepad.Rumble(e.device, amount, amount)
	e.rumbleActive = active
}

func (e *Engine) disconnect() {
	e.releaseAllHeldActions()
	if e.windowSwitching {
		_ = e.finishWindowSwitch()
	}
	if e.voiceHeld != 0 {
		_ = e.voiceReleased()
	}
	if e.device != "" {
		_ = e.gamepad.Rumble(e.device, 0, 0)
	}
	e.device = ""
	e.previousButtons = 0
	e.voiceHeld = 0
	e.previousLeftTrigger = false
	e.previousRightTrigger = false
}

func (e *Engine) shutdown() {
	e.disconnect()
}
