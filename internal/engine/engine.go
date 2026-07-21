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
	"github.com/wangzhigang1999/couchpilot/internal/usage"
)

var ErrExitRequested = errors.New("emergency exit requested")

const (
	composeDeleteRepeatDelay    = 320 * time.Millisecond
	composeDeleteRepeatInterval = 75 * time.Millisecond
)

type Engine struct {
	settings   config.Settings
	gamepad    core.Gamepad
	desktop    core.Desktop
	clock      core.Clock
	resolver   *Resolver
	logger     *log.Logger
	verbose    bool
	usage      usage.Recorder
	strategyID string
	trace      traceState

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
	moveRemainder        [2]float64
}

func New(settings config.Settings, gamepad core.Gamepad, desktop core.Desktop, verbose bool, output io.Writer) *Engine {
	if output == nil {
		output = io.Discard
	}
	resolver := NewResolver(settings.Bindings)
	return &Engine{
		settings:    settings,
		gamepad:     gamepad,
		desktop:     desktop,
		clock:       core.RealClock{},
		resolver:    resolver,
		logger:      log.New(output, "", log.LstdFlags),
		verbose:     verbose,
		heldActions: make(map[core.Button]core.Action),
		strategyID:  strategyID(settings, resolver.BindingInventory()),
		trace:       newTraceState(),
	}
}

// SetUsageRecorder attaches a non-blocking observation sink. It is intended to
// be called during worker setup, before Run starts.
func (e *Engine) SetUsageRecorder(recorder usage.Recorder) {
	e.usage = recorder
}

func (e *Engine) BindingInventory() []usage.BindingDefinition {
	return e.resolver.BindingInventory()
}

// StrategyRevision identifies the effective bindings and gesture semantics
// that produced this engine's trace observations.
func (e *Engine) StrategyRevision() string {
	return e.strategyID
}

func (e *Engine) UsageControls() []string {
	return append([]string(nil), usageControls...)
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
			e.trace.lastAt = frameStarted
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
	e.trace.lastAt = now
	e.expireCompose(now)
	leftTriggerWasActive := e.previousLeftTrigger
	e.logEdges(state, now)
	e.observeStickEdges(state, now)
	if leftTriggerWasActive && state.LeftTrigger <= 0.08 && e.windowSwitching {
		if err := e.finishWindowSwitch(now, "trigger_released", usage.OutcomeNone); err != nil {
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

func (e *Engine) movePointer(state core.State, dt float64, now time.Time) error {
	x, y := state.LeftX, -state.LeftY
	magnitude := math.Hypot(x, y)
	if magnitude < 1e-4 {
		e.moveRemainder = [2]float64{}
		return nil
	}
	multiplier := 1.0
	if state.LeftTrigger > 0.08 {
		e.markPointerModifier("lt")
		amount := math.Min(1, (state.LeftTrigger-0.08)/0.92)
		multiplier = 1 - amount*(1-e.settings.PrecisionSpeedMultiplier)
	} else if state.RightTrigger > 0.08 {
		e.markPointerModifier("rt")
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
	e.clearCompose("pointer_movement", now)
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

var usageControls = []string{
	"a", "b", "back", "back+start",
	"dpad_down", "dpad_left", "dpad_right", "dpad_up",
	"l3", "lb", "left_stick", "lt",
	"r3", "rb", "right_stick", "rt",
	"start", "x", "y",
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
	e.finishButtonHolds(released, now, "released")
	for _, item := range gestures {
		if released&item.button == 0 {
			continue
		}
		e.stopRepeat(item.button, now, "released")
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
	e.startButtonHolds(pressed, now, profile, foregroundApp)
	for _, item := range unboundSystemButtons {
		if pressed&item.button == 0 {
			continue
		}
		resolved := ResolvedBinding{
			ActiveProfile: profile,
			Gesture:       item.control,
			Resolution:    usage.ResolutionUnbound,
		}
		e.updateButtonHold(item.button, resolved)
		e.recordResolved(now, item.control, resolved, usage.OutcomeNone, foregroundApp, state, simultaneous)
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
			e.clearCompose("other_control", now)
		}
		baseGesture := gesture
		leftActive := !composeSubmit && state.LeftTrigger > 0.08
		rightActive := !composeSubmit && state.RightTrigger > 0.08
		var leftCandidate, rightCandidate ResolvedBinding
		selectedModifier := ""
		if leftActive {
			leftCandidate = e.resolver.ResolveDetailed(profile, "lt+"+baseGesture)
			if leftCandidate.Resolution == usage.ResolutionBound {
				gesture = leftCandidate.Gesture
				selectedModifier = "lt"
			}
		}
		if rightActive {
			rightCandidate = e.resolver.ResolveDetailed(profile, "rt+"+baseGesture)
			// Preserve the existing LT-first resolver behavior when both
			// triggers are active, even when only the RT candidate is bound.
			if !leftActive && rightCandidate.Resolution == usage.ResolutionBound {
				gesture = rightCandidate.Gesture
				selectedModifier = "rt"
			}
		}
		resolved := e.resolver.ResolveDetailed(profile, gesture)
		e.recordChordProbes(now, item.gesture, resolved, foregroundApp, state,
			leftActive, rightActive, leftCandidate, rightCandidate, selectedModifier)
		e.updateButtonHold(item.button, resolved)
		if resolved.Resolution != usage.ResolutionBound {
			e.recordResolved(now, item.gesture, resolved, usage.OutcomeNone, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearCompose("submit_unavailable", now)
			}
			continue
		}
		action := resolved.Action
		if composeDelete {
			if err := e.desktop.Perform(action); err != nil {
				e.recordResolved(now, item.gesture, resolved, usage.OutcomeFailure, foregroundApp, state, simultaneous)
				e.clearComposeWithOutcome("delete_dispatch_failed", now, usage.OutcomeFailure)
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.recordResolved(now, item.gesture, resolved, usage.OutcomeSuccess, foregroundApp, state, simultaneous)
			e.trace.compose.deletes++
			e.startRepeat(item.button, item.gesture, resolved.Gesture, action, profile, foregroundApp, now)
			e.actionHaptic(action)
			if e.verbose {
				e.logger.Printf("%s/%s -> %s", profile, gesture, action)
			}
			return nil
		}
		if action == core.Voice {
			if err := e.voicePressed(item.button); err != nil {
				e.recordResolved(now, item.gesture, resolved, usage.OutcomeFailure, foregroundApp, state, simultaneous)
				if composeSubmit {
					e.clearComposeWithOutcome("submit_dispatch_failed", now, usage.OutcomeFailure)
				}
				return err
			}
			e.recordResolved(now, item.gesture, resolved, usage.OutcomeSuccess, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearComposeWithOutcome("submit_succeeded", now, usage.OutcomeSuccess)
			}
			e.armCompose(profile, foregroundApp, now)
			continue
		}
		if down, up, held := heldActionPair(action); held {
			if err := e.desktop.Perform(down); err != nil {
				e.recordResolved(now, item.gesture, resolved, usage.OutcomeFailure, foregroundApp, state, simultaneous)
				if composeSubmit {
					e.clearComposeWithOutcome("submit_dispatch_failed", now, usage.OutcomeFailure)
				}
				return fmt.Errorf("perform %s: %w", action, err)
			}
			e.recordResolved(now, item.gesture, resolved, usage.OutcomeSuccess, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearComposeWithOutcome("submit_succeeded", now, usage.OutcomeSuccess)
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
				e.startWindowTrace(now, profile, foregroundApp, action)
			case core.WindowNext:
				performedAction = core.WindowCycleNext
				e.windowSwitching = true
				e.startWindowTrace(now, profile, foregroundApp, action)
			}
		}
		if err := e.desktop.Perform(performedAction); err != nil {
			if e.windowSwitching {
				_ = e.finishWindowSwitch(now, "cycle_failure", usage.OutcomeFailure)
			}
			e.recordResolved(now, item.gesture, resolved, usage.OutcomeFailure, foregroundApp, state, simultaneous)
			if composeSubmit {
				e.clearComposeWithOutcome("submit_dispatch_failed", now, usage.OutcomeFailure)
			}
			return fmt.Errorf("perform %s: %w", action, err)
		}
		e.recordResolved(now, item.gesture, resolved, usage.OutcomeSuccess, foregroundApp, state, simultaneous)
		if composeSubmit {
			e.clearComposeWithOutcome("submit_succeeded", now, usage.OutcomeSuccess)
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

func (e *Engine) armCompose(profile, foregroundApp string, now time.Time) {
	if _, found := e.resolver.Resolve(profile, "voice+a"); !found {
		e.clearCompose("unsupported", now)
		return
	}
	e.finishComposeTrace(now, "restarted", usage.OutcomeNone)
	e.composeProfile = profile
	e.composeUntil = now.Add(time.Duration(e.settings.VoiceSubmitTimeoutSeconds * float64(time.Second)))
	e.trace.compose = composeTrace{
		active:        true,
		started:       now,
		foregroundApp: foregroundApp,
		activeProfile: profile,
	}
	if e.verbose {
		e.logger.Printf("%s voice compose armed; A submits, B deletes", profile)
	}
}

func (e *Engine) composeActive(profile string, now time.Time) bool {
	if e.composeProfile == "" {
		return false
	}
	if profile != e.composeProfile {
		e.clearCompose("profile_changed", now)
		return false
	}
	if !now.Before(e.composeUntil) {
		e.clearCompose("timeout", now)
		return false
	}
	return true
}

func (e *Engine) expireCompose(now time.Time) {
	if e.composeProfile != "" && !now.Before(e.composeUntil) {
		e.clearCompose("timeout", now)
	}
}

func (e *Engine) clearCompose(reason string, now time.Time) {
	e.clearComposeWithOutcome(reason, now, usage.OutcomeNone)
}

func (e *Engine) clearComposeWithOutcome(reason string, now time.Time, outcome usage.Outcome) {
	if e.composeProfile == "" {
		return
	}
	if e.verbose && reason != "" {
		e.logger.Printf("%s voice compose cleared: %s", e.composeProfile, reason)
	}
	e.finishRepeatTrace(now, reason, outcome)
	e.finishComposeTrace(now, reason, outcome)
	e.composeProfile = ""
	e.composeUntil = time.Time{}
	e.stopRepeat(0, now, reason)
}

func (e *Engine) startRepeat(button core.Button, physicalGesture, gesture string, action core.Action, profile, foregroundApp string, now time.Time) {
	e.repeatButton = button
	e.repeatAction = action
	e.repeatNext = now.Add(composeDeleteRepeatDelay)
	e.startRepeatTrace(now, profile, foregroundApp, physicalGesture, gesture, action)
}

func (e *Engine) stopRepeat(button core.Button, now time.Time, reason string) {
	if button != 0 && e.repeatButton != button {
		return
	}
	e.finishRepeatTrace(now, reason, usage.OutcomeSuccess)
	e.repeatButton = 0
	e.repeatAction = ""
	e.repeatNext = time.Time{}
}

func (e *Engine) repeatHeldAction(state core.State, now time.Time) error {
	if e.repeatButton == 0 || state.Buttons&e.repeatButton == 0 || now.Before(e.repeatNext) {
		return nil
	}
	profile, _ := e.desktop.ForegroundContext()
	if profile != e.composeProfile {
		e.clearCompose("profile_changed", now)
		return nil
	}
	if err := e.desktop.Perform(e.repeatAction); err != nil {
		e.clearComposeWithOutcome("repeat_dispatch_failure", now, usage.OutcomeFailure)
		return fmt.Errorf("repeat %s: %w", e.repeatAction, err)
	}
	e.trace.repeat.count++
	e.trace.compose.repeats++
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

func (e *Engine) finishWindowSwitch(now time.Time, reason string, sessionOutcome usage.Outcome) error {
	if !e.windowSwitching {
		return nil
	}
	e.windowSwitching = false
	if err := e.desktop.Perform(core.WindowCycleCommit); err != nil {
		e.finishWindowTrace(now, reason, usage.OutcomeFailure)
		return err
	}
	if sessionOutcome == usage.OutcomeNone {
		sessionOutcome = usage.OutcomeSuccess
	}
	e.finishWindowTrace(now, reason, sessionOutcome)
	e.actionHaptic(core.WindowCycleCommit)
	return nil
}

func (e *Engine) logEdges(state core.State, now time.Time) {
	left := state.LeftTrigger > 0.08
	right := state.RightTrigger > 0.08
	if left && !e.previousLeftTrigger {
		profile, foregroundApp := e.foregroundContext()
		e.startTrigger("lt", now, profile, foregroundApp)
		e.recordObserved(now, "lt", usage.GestureTriggerHold, "modifier")
		e.recordLateModifierProbes("lt", state, now, profile, foregroundApp)
	} else if !left && e.previousLeftTrigger {
		e.finishTrigger("lt", now, "released")
	}
	if right && !e.previousRightTrigger {
		profile, foregroundApp := e.foregroundContext()
		e.startTrigger("rt", now, profile, foregroundApp)
		e.recordObserved(now, "rt", usage.GestureTriggerHold, "modifier")
		e.recordLateModifierProbes("rt", state, now, profile, foregroundApp)
	} else if !right && e.previousRightTrigger {
		e.finishTrigger("rt", now, "released")
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
		e.recordObserved(now, "left_stick", usage.GestureAnalogActivation, "pointer")
	}
	if right && !e.previousRightStick {
		axis := "mixed_axes"
		if math.Abs(state.RightX) < 1e-4 {
			axis = "vertical_only"
		} else if math.Abs(state.RightY) < 1e-4 {
			axis = "horizontal_only"
		}
		e.recordObserved(now, "right_stick", usage.GestureAnalogActivation, axis)
	}
	e.previousLeftStick = left
	e.previousRightStick = right
}

func (e *Engine) activeProfile() string {
	profile, _ := e.foregroundContext()
	return profile
}

func (e *Engine) foregroundContext() (string, string) {
	profile, processName := e.desktop.ForegroundContext()
	if profile == "" {
		profile = "default"
	}
	return profile, processName
}

func (e *Engine) recordResolved(now time.Time, control string, resolved ResolvedBinding, outcome usage.Outcome, foregroundApp string, state core.State, simultaneous bool) {
	e.emit(usage.Observation{
		At:              now,
		Kind:            usage.EventInputAttempt,
		ForegroundApp:   foregroundApp,
		ActiveProfile:   resolved.ActiveProfile,
		BindingProfile:  resolved.BindingProfile,
		Control:         control,
		PhysicalGesture: physicalGestureForAttempt(control, resolved, state),
		Gesture:         resolved.Gesture,
		GestureKind:     gestureKind(resolved.Gesture),
		Action:          string(resolved.Action),
		Resolution:      resolved.Resolution,
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

func (e *Engine) recordObserved(now time.Time, control string, kind usage.GestureKind, flags string) {
	if e.usage == nil {
		return
	}
	profile, foregroundApp := e.foregroundContext()
	e.emit(usage.Observation{
		At:              now,
		Kind:            usage.EventPhysicalActivation,
		ForegroundApp:   foregroundApp,
		ActiveProfile:   profile,
		Control:         control,
		PhysicalGesture: control,
		Gesture:         control,
		GestureKind:     kind,
		Resolution:      usage.ResolutionObserved,
		Outcome:         usage.OutcomeNone,
		Flags:           flags,
	})
}

func (e *Engine) recordSystemExit(now time.Time) {
	if e.usage == nil {
		return
	}
	profile, foregroundApp := e.foregroundContext()
	e.emit(usage.Observation{
		At:              now,
		Kind:            usage.EventPhysicalActivation,
		ForegroundApp:   foregroundApp,
		ActiveProfile:   profile,
		Control:         "back+start",
		PhysicalGesture: "back+start",
		Gesture:         "back+start",
		GestureKind:     usage.GestureSystemHold,
		Action:          "emergency_exit",
		Resolution:      usage.ResolutionSystem,
		Outcome:         usage.OutcomeSuccess,
		DurationBucket:  durationBucket(now.Sub(e.exitComboStarted)),
		Reason:          "threshold_reached",
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
	now := e.trace.lastAt
	if now.IsZero() {
		now = e.clock.Now()
	}
	e.traceDisconnect(now)
	e.releaseAllHeldActions()
	e.clearCompose("disconnect", now)
	if e.windowSwitching {
		_ = e.finishWindowSwitch(now, "disconnect", usage.OutcomeNone)
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
