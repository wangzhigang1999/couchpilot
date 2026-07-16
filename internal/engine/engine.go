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

const (
	composeDeleteRepeatDelay    = 320 * time.Millisecond
	composeDeleteRepeatInterval = 75 * time.Millisecond
)

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
	rumbleLeft           uint16
	rumbleRight          uint16
	rumbleSentLeft       uint16
	rumbleSentRight      uint16
	windowSwitching      bool
	composeProfile       string
	composeUntil         time.Time
	repeatButton         core.Button
	repeatAction         core.Action
	repeatNext           time.Time
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
			e.pulseHaptic(28000, 20000, 120*time.Millisecond)
			e.updateRumble(frameStarted)
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
	e.expireCompose(now)
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
	if err := e.buttons(state, now); err != nil {
		return err
	}
	if err := e.repeatHeldAction(state, now); err != nil {
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
	e.clearCompose("pointer movement")
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

func (e *Engine) buttons(state core.State, now time.Time) error {
	pressed := core.Button(uint16(state.Buttons) &^ uint16(e.previousButtons))
	released := core.Button(uint16(e.previousButtons) &^ uint16(state.Buttons))
	for _, item := range gestures {
		if released&item.button == 0 {
			continue
		}
		e.stopRepeat(item.button)
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
		composeActive := e.composeActive(profile, now)
		noTrigger := state.LeftTrigger <= 0.08 && state.RightTrigger <= 0.08
		composeSubmit := false
		composeDelete := false
		if composeActive && noTrigger && gesture == "a" {
			gesture = "voice+a"
			composeSubmit = true
			e.clearCompose("submitted")
		} else if composeActive && gesture == "b" {
			gesture = "voice+b"
			composeDelete = true
		} else if composeActive && gesture != "y" {
			e.clearCompose("another control was used")
		}
		if !composeSubmit && state.LeftTrigger > 0.08 {
			if _, found := e.resolver.Resolve(profile, "lt+"+gesture); found {
				gesture = "lt+" + gesture
			}
		} else if !composeSubmit && state.RightTrigger > 0.08 {
			if _, found := e.resolver.Resolve(profile, "rt+"+gesture); found {
				gesture = "rt+" + gesture
			}
		}
		action, found := e.resolver.Resolve(profile, gesture)
		if !found {
			continue
		}
		if composeDelete {
			if err := e.desktop.Perform(action); err != nil {
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.startRepeat(item.button, action, now)
			e.actionHaptic(action)
			if e.verbose {
				e.logger.Printf("%s/%s -> %s", profile, gesture, action)
			}
			return nil
		}
		if action == core.Voice {
			if err := e.voicePressed(item.button); err != nil {
				return err
			}
			e.armCompose(profile, now)
			continue
		}
		if down, up, held := heldActionPair(action); held {
			if err := e.desktop.Perform(down); err != nil {
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.heldActions[item.button] = up
			e.actionHaptic(action)
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
		e.actionHaptic(performedAction)
		if e.verbose {
			e.logger.Printf("%s/%s -> %s", profile, gesture, action)
		}
		if composeSubmit {
			return nil
		}
	}
	return nil
}

func (e *Engine) armCompose(profile string, now time.Time) {
	if _, found := e.resolver.Resolve(profile, "voice+a"); !found {
		e.clearCompose("")
		return
	}
	e.composeProfile = profile
	e.composeUntil = now.Add(time.Duration(e.settings.VoiceSubmitTimeoutSeconds * float64(time.Second)))
	if e.verbose {
		e.logger.Printf("%s voice compose armed; A submits, B deletes", profile)
	}
}

func (e *Engine) composeActive(profile string, now time.Time) bool {
	if e.composeProfile == "" {
		return false
	}
	if profile != e.composeProfile || !now.Before(e.composeUntil) {
		e.clearCompose("profile changed or timeout elapsed")
		return false
	}
	return true
}

func (e *Engine) expireCompose(now time.Time) {
	if e.composeProfile != "" && !now.Before(e.composeUntil) {
		e.clearCompose("timeout elapsed")
	}
}

func (e *Engine) clearCompose(reason string) {
	if e.composeProfile == "" {
		return
	}
	if e.verbose && reason != "" {
		e.logger.Printf("%s voice compose cleared: %s", e.composeProfile, reason)
	}
	e.composeProfile = ""
	e.composeUntil = time.Time{}
	e.stopRepeat(0)
}

func (e *Engine) startRepeat(button core.Button, action core.Action, now time.Time) {
	e.repeatButton = button
	e.repeatAction = action
	e.repeatNext = now.Add(composeDeleteRepeatDelay)
}

func (e *Engine) stopRepeat(button core.Button) {
	if button != 0 && e.repeatButton != button {
		return
	}
	e.repeatButton = 0
	e.repeatAction = ""
	e.repeatNext = time.Time{}
}

func (e *Engine) repeatHeldAction(state core.State, now time.Time) error {
	if e.repeatButton == 0 || state.Buttons&e.repeatButton == 0 || now.Before(e.repeatNext) {
		return nil
	}
	if e.desktop.ForegroundProfile() != e.composeProfile {
		e.clearCompose("profile changed while deleting")
		return nil
	}
	if err := e.desktop.Perform(e.repeatAction); err != nil {
		return fmt.Errorf("repeat %s: %w", e.repeatAction, err)
	}
	e.repeatNext = now.Add(composeDeleteRepeatInterval)
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
	var err error
	switch e.settings.VoiceMode {
	case "tap":
		err = e.desktop.Perform(core.VoiceTap)
	case "toggle_while_held":
		e.voiceHeld |= button
		err = e.desktop.Perform(core.VoiceTap)
	case "hold":
		e.voiceHeld |= button
		err = e.desktop.Perform(core.VoiceDown)
	}
	if err == nil {
		e.actionHaptic(core.VoiceTap)
	}
	return err
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
	if err := e.desktop.Perform(core.WindowCycleCommit); err != nil {
		return err
	}
	e.actionHaptic(core.WindowCycleCommit)
	return nil
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
	if e.device == "" {
		return
	}
	left, right := uint16(0), uint16(0)
	if e.settings.HapticsEnabled && now.Before(e.rumbleUntil) {
		left, right = e.rumbleLeft, e.rumbleRight
	}
	if left == e.rumbleSentLeft && right == e.rumbleSentRight {
		return
	}
	if err := e.gamepad.Rumble(e.device, left, right); err == nil {
		e.rumbleSentLeft, e.rumbleSentRight = left, right
	}
}

func (e *Engine) actionHaptic(action core.Action) {
	switch action {
	case core.ClickLeft, core.ClickRight:
		e.pulseHaptic(9000, 22000, 45*time.Millisecond)
	case core.ArrowUp, core.ArrowDown, core.ArrowLeft, core.ArrowRight,
		core.TabPrevious, core.TabNext, core.PageUp, core.PageDown,
		core.CodexPreviousTask, core.CodexNextTask,
		core.ChromePreviousTab, core.ChromeNextTab:
		e.pulseHaptic(12000, 26000, 60*time.Millisecond)
	case core.WindowCyclePrevious, core.WindowCycleNext, core.WindowPrevious, core.WindowNext:
		e.pulseHaptic(32000, 22000, 75*time.Millisecond)
	case core.WindowCycleCommit:
		e.pulseHaptic(42000, 32000, 110*time.Millisecond)
	case core.VoiceTap:
		e.pulseHaptic(26000, 30000, 90*time.Millisecond)
	default:
		e.pulseHaptic(20000, 24000, 70*time.Millisecond)
	}
}

func (e *Engine) pulseHaptic(left, right uint16, duration time.Duration) {
	if !e.settings.HapticsEnabled || e.settings.HapticStrength <= 0 {
		return
	}
	scale := func(amount uint16) uint16 {
		value := math.Round(float64(amount) * e.settings.HapticStrength)
		if value > 65535 {
			value = 65535
		}
		return uint16(value)
	}
	e.rumbleLeft, e.rumbleRight = scale(left), scale(right)
	e.rumbleUntil = e.clock.Now().Add(duration)
}

func (e *Engine) disconnect() {
	e.releaseAllHeldActions()
	e.clearCompose("controller disconnected")
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
	e.rumbleLeft, e.rumbleRight = 0, 0
	e.rumbleSentLeft, e.rumbleSentRight = 0, 0
	e.rumbleUntil = time.Time{}
}

func (e *Engine) shutdown() {
	e.disconnect()
}
