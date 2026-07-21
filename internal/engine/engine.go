package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

var ErrExitRequested = errors.New("emergency exit requested")

const (
	composeDeleteRepeatDelay    = 320 * time.Millisecond
	composeDeleteRepeatInterval = 75 * time.Millisecond
)

type Engine struct {
	settings       config.Settings
	gamepad        core.Gamepad
	desktop        core.Desktop
	smoothScroller core.SmoothScroller
	clock          core.Clock
	resolver       *Resolver
	logger         *log.Logger
	verbose        bool
	traceSink      trace.Sink

	device               core.DeviceID
	previousButtons      core.Button
	previousLeftTrigger  bool
	previousRightTrigger bool
	previousLeftStick    bool
	previousRightStick   bool
	voiceHeld            core.Button
	exitComboStarted     time.Time
	exitComboRecorded    bool
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
	smoothScrollVelocity float64
	smoothScrollActive   bool
	moveRemainder        [2]float64
	frameContextLoaded   bool
	frameProfile         string
	frameProcessName     string
}

func New(settings config.Settings, gamepad core.Gamepad, desktop core.Desktop, verbose bool, output io.Writer) *Engine {
	if output == nil {
		output = io.Discard
	}
	result := &Engine{
		settings:    settings,
		gamepad:     gamepad,
		desktop:     desktop,
		clock:       core.RealClock{},
		resolver:    NewResolver(settings.Bindings),
		logger:      log.New(output, "", log.LstdFlags),
		verbose:     verbose,
		heldActions: make(map[core.Button]core.Action),
	}
	result.smoothScroller, _ = desktop.(core.SmoothScroller)
	return result
}

// SetTraceSink attaches a non-owning diagnostic sink during worker setup.
func (e *Engine) SetTraceSink(sink trace.Sink) {
	e.traceSink = sink
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
			if e.verbose {
				if diagnostics, ok := e.gamepad.(core.GamepadDiagnostics); ok {
					if detail, err := diagnostics.Diagnostic(device); err == nil {
						e.logger.Printf("controller diagnostic: %s", detail)
					} else {
						e.logger.Printf("controller diagnostic unavailable: %v", err)
					}
				}
			}
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
	// Consume physical edges even when an action fails. The worker exits on
	// those errors, but keeping the state transition atomic also prevents a
	// caller that inspects the failure from recording the same press twice.
	defer func() { e.previousButtons = state.Buttons }()
	e.frameContextLoaded = false
	e.expireCompose(now)
	leftTriggerWasActive := e.previousLeftTrigger
	e.logEdges(state, now)
	e.observeStickEdges(state, now)
	if leftTriggerWasActive && state.LeftTrigger <= 0.08 && e.windowSwitching {
		if err := e.finishWindowSwitch(); err != nil {
			return err
		}
	}
	combo := core.Back | core.Start
	if state.Buttons&combo == combo {
		if e.exitComboStarted.IsZero() {
			e.exitComboStarted = now
		} else if !e.exitComboRecorded && now.Sub(e.exitComboStarted).Seconds() >= e.settings.ExitHoldSeconds {
			e.exitComboRecorded = true
			e.recordSystemExit(now)
			return ErrExitRequested
		}
	} else {
		e.exitComboStarted = time.Time{}
		e.exitComboRecorded = false
	}
	if err := e.movePointer(state, dt, now); err != nil {
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
		if e.settings.ControllerIndex < len(devices) {
			return devices[e.settings.ControllerIndex], true, nil
		}
		return "", false, nil
	}
	if len(devices) == 0 {
		return "", false, nil
	}
	return devices[0], true, nil
}

func (e *Engine) movePointer(state core.State, dt float64, now time.Time) error {
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
	e.clearCompose("pointer_movement")
	return e.desktop.MovePointer(dx, dy)
}

func (e *Engine) scroll(state core.State, dt float64) error {
	if e.smoothScroller != nil {
		return e.scrollSmooth(state.RightY, dt)
	}
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

func (e *Engine) scrollSmooth(axis, dt float64) error {
	if dt < 0 {
		dt = 0
	}
	targetVelocity := axis * e.settings.ScrollUnitsPerSecond
	response := 24.0
	if math.Abs(targetVelocity) < 1e-4 {
		// A short decay gives the stick a trackpad-like release without the long
		// momentum tail that would make precise UI scrolling hard to stop.
		response = 10
	}
	alpha := 1 - math.Exp(-response*dt)
	e.smoothScrollVelocity += (targetVelocity - e.smoothScrollVelocity) * alpha
	if math.Abs(targetVelocity) < 1e-4 && math.Abs(e.smoothScrollVelocity) < 2 {
		e.smoothScrollVelocity = 0
	}
	if e.smoothScrollVelocity == 0 {
		if !e.smoothScrollActive {
			return nil
		}
		e.smoothScrollActive = false
		return e.smoothScroller.ScrollSmooth(0, core.SmoothScrollEnded)
	}
	phase := core.SmoothScrollChanged
	if !e.smoothScrollActive {
		e.smoothScrollActive = true
		phase = core.SmoothScrollBegan
	}
	return e.smoothScroller.ScrollSmooth(e.smoothScrollVelocity*dt, phase)
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

var unboundSystemButtons = []struct {
	button  core.Button
	control string
}{
	{core.Back, "back"},
	{core.Start, "start"},
}

func (e *Engine) buttons(state core.State, now time.Time) error {
	pressed := core.Button(uint16(state.Buttons) &^ uint16(e.previousButtons))
	released := core.Button(uint16(e.previousButtons) &^ uint16(state.Buttons))
	if e.verbose && (pressed != 0 || released != 0) {
		e.logger.Printf("buttons: state=0x%04X pressed=0x%04X released=0x%04X", uint16(state.Buttons), uint16(pressed), uint16(released))
	}
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
	profile, foregroundApp := e.foregroundContext()
	simultaneous := pressedButtonCount(pressed) > 1
	for _, item := range unboundSystemButtons {
		if pressed&item.button == 0 {
			continue
		}
		resolved := ResolvedBinding{
			ActiveProfile: profile,
			Gesture:       item.control,
			Resolution:    BindingUnbound,
		}
		e.recordResolved(now, item.control, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
	}
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
		} else if composeActive && gesture == "b" {
			gesture = "voice+b"
			composeDelete = true
		} else if composeActive && gesture != "y" {
			e.clearCompose("other_control")
		}
		baseGesture := gesture
		leftActive := !composeSubmit && state.LeftTrigger > 0.08
		rightActive := !composeSubmit && state.RightTrigger > 0.08
		if leftActive {
			leftCandidate := e.resolver.ResolveDetailed(profile, "lt+"+baseGesture)
			if leftCandidate.Resolution == BindingBound {
				gesture = leftCandidate.Gesture
			}
		}
		if rightActive {
			rightCandidate := e.resolver.ResolveDetailed(profile, "rt+"+baseGesture)
			// Preserve the existing LT-first resolver behavior when both
			// triggers are active, even when only the RT candidate is bound.
			if !leftActive && rightCandidate.Resolution == BindingBound {
				gesture = rightCandidate.Gesture
			}
		}
		resolved := e.resolver.ResolveDetailed(profile, gesture)
		if resolved.Resolution != BindingBound {
			e.recordResolved(now, item.gesture, resolved, trace.NoOutcome, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_unavailable")
			}
			continue
		}
		action := resolved.Action
		if composeDelete {
			if err := e.desktop.Perform(action); err != nil {
				e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
				e.clearCompose("delete_dispatch_failed")
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
			e.startRepeat(item.button, action, now)
			e.actionHaptic(action)
			if e.verbose {
				e.logger.Printf("%s/%s -> %s", profile, gesture, action)
			}
			return nil
		}
		if action == core.Voice {
			if err := e.voicePressed(item.button); err != nil {
				e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
				if composeSubmit {
					e.clearCompose("submit_dispatch_failed")
				}
				return err
			}
			e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_succeeded")
			}
			e.armCompose(profile, now)
			continue
		}
		if down, up, held := heldActionPair(action); held {
			if err := e.desktop.Perform(down); err != nil {
				e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
				if composeSubmit {
					e.clearCompose("submit_dispatch_failed")
				}
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_succeeded")
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
			e.recordResolved(now, item.gesture, resolved, trace.Failure, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_dispatch_failed")
			}
			return fmt.Errorf("perform %s: %w", action, err)
		}
		e.recordResolved(now, item.gesture, resolved, trace.Success, foregroundApp, state, simultaneous)
		if composeSubmit {
			e.clearCompose("submit_succeeded")
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
		e.clearCompose("unsupported")
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
	if profile != e.composeProfile {
		e.clearCompose("profile_changed")
		return false
	}
	if !now.Before(e.composeUntil) {
		e.clearCompose("timeout")
		return false
	}
	return true
}

func (e *Engine) expireCompose(now time.Time) {
	if e.composeProfile != "" && !now.Before(e.composeUntil) {
		e.clearCompose("timeout")
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
	profile, _ := e.foregroundContext()
	if profile != e.composeProfile {
		e.clearCompose("profile_changed")
		return nil
	}
	if err := e.desktop.Perform(e.repeatAction); err != nil {
		e.clearCompose("repeat_dispatch_failure")
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

func (e *Engine) logEdges(state core.State, now time.Time) {
	left := state.LeftTrigger > 0.08
	right := state.RightTrigger > 0.08
	if left && !e.previousLeftTrigger {
		e.recordObserved(now, "lt", "modifier")
	}
	if right && !e.previousRightTrigger {
		e.recordObserved(now, "rt", "modifier")
	}
	if e.verbose && left != e.previousLeftTrigger {
		e.logger.Printf("LT precision: %t", left)
	}
	if e.verbose && right != e.previousRightTrigger {
		e.logger.Printf("RT boost: %t", right)
	}
	e.previousLeftTrigger = left
	e.previousRightTrigger = right
}

func (e *Engine) observeStickEdges(state core.State, now time.Time) {
	left := math.Hypot(state.LeftX, state.LeftY) >= 1e-4
	right := math.Hypot(state.RightX, state.RightY) >= 1e-4
	if left && !e.previousLeftStick {
		e.recordObserved(now, "left_stick", "pointer")
	}
	if right && !e.previousRightStick {
		axis := "mixed_axes"
		if math.Abs(state.RightX) < 1e-4 {
			axis = "vertical_only"
		} else if math.Abs(state.RightY) < 1e-4 {
			axis = "horizontal_only"
		}
		e.recordObserved(now, "right_stick", axis)
	}
	e.previousLeftStick = left
	e.previousRightStick = right
}

func (e *Engine) foregroundContext() (string, string) {
	if e.frameContextLoaded {
		return e.frameProfile, e.frameProcessName
	}
	profile, processName := e.desktop.ForegroundContext()
	if profile == "" {
		profile = "default"
	}
	e.frameContextLoaded = true
	e.frameProfile = profile
	e.frameProcessName = processName
	return profile, processName
}

func (e *Engine) recordResolved(now time.Time, control string, resolved ResolvedBinding, outcome trace.Outcome, foregroundApp string, state core.State, simultaneous bool) {
	e.emit(trace.Fact{
		At:              now,
		Kind:            trace.InputAttempt,
		ForegroundApp:   foregroundApp,
		ActiveProfile:   resolved.ActiveProfile,
		BindingProfile:  resolved.BindingProfile,
		Control:         control,
		PhysicalGesture: physicalGestureForAttempt(control, resolved, state),
		Gesture:         resolved.Gesture,
		Action:          string(resolved.Action),
		Resolution:      trace.Resolution(resolved.Resolution),
		Outcome:         outcome,
		Flags: stableFlags(
			flagIf(state.LeftTrigger > 0.08, "lt_active"),
			flagIf(state.RightTrigger > 0.08, "rt_active"),
			flagIf(math.Hypot(state.LeftX, state.LeftY) >= 1e-4, "left_stick_active"),
			flagIf(math.Hypot(state.RightX, state.RightY) >= 1e-4, "right_stick_active"),
			flagIf(state.LeftTrigger > 0.08 && state.RightTrigger > 0.08, "dual_trigger"),
			flagIf(simultaneous, "simultaneous_buttons"),
		),
	})
}

func (e *Engine) recordObserved(now time.Time, control, flags string) {
	if e.traceSink == nil {
		return
	}
	profile, foregroundApp := e.foregroundContext()
	e.emit(trace.Fact{
		At:              now,
		Kind:            trace.PhysicalActivation,
		ForegroundApp:   foregroundApp,
		ActiveProfile:   profile,
		Control:         control,
		PhysicalGesture: control,
		Gesture:         control,
		Resolution:      trace.Observed,
		Outcome:         trace.NoOutcome,
		Flags:           flags,
	})
}

func (e *Engine) recordSystemExit(now time.Time) {
	if e.traceSink == nil {
		return
	}
	profile, foregroundApp := e.foregroundContext()
	e.emit(trace.Fact{
		At:              now,
		Kind:            trace.PhysicalActivation,
		ForegroundApp:   foregroundApp,
		ActiveProfile:   profile,
		Control:         "back+start",
		PhysicalGesture: "back+start",
		Gesture:         "back+start",
		Action:          "emergency_exit",
		Resolution:      trace.System,
		Outcome:         trace.Success,
	})
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
	if capabilities, ok := e.gamepad.(core.GamepadCapabilities); ok && !capabilities.HapticsSupported(e.device) {
		e.rumbleSentLeft, e.rumbleSentRight = left, right
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
		core.CodexPreviousTask, core.CodexNextTask:
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
	if e.smoothScrollActive && e.smoothScroller != nil {
		_ = e.smoothScroller.ScrollSmooth(0, core.SmoothScrollEnded)
	}
	e.smoothScrollActive = false
	e.smoothScrollVelocity = 0
	e.releaseAllHeldActions()
	e.clearCompose("disconnect")
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
	e.previousLeftStick = false
	e.previousRightStick = false
	e.exitComboStarted = time.Time{}
	e.exitComboRecorded = false
	e.rumbleLeft, e.rumbleRight = 0, 0
	e.rumbleSentLeft, e.rumbleSentRight = 0, 0
	e.rumbleUntil = time.Time{}
}

func (e *Engine) shutdown() {
	e.disconnect()
}
