package engine

import (
	"math"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

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

func (e *Engine) recordResolved(now time.Time, control string, resolved mapping.ResolvedBinding, outcome trace.Outcome, foregroundApp string, state core.State, simultaneous bool) {
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
	if e.options.HapticsEnabled && now.Before(e.rumbleUntil) {
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

func (e *Engine) actionHaptic(action string) {
	switch action {
	case string(core.ClickLeft), string(core.ClickRight):
		e.pulseHaptic(9000, 22000, 45*time.Millisecond)
	case string(core.ArrowUp), string(core.ArrowDown), string(core.ArrowLeft), string(core.ArrowRight),
		string(core.TabPrevious), string(core.TabNext), string(core.PageUp), string(core.PageDown),
		string(core.CodexPreviousTask), string(core.CodexNextTask):
		e.pulseHaptic(12000, 26000, 60*time.Millisecond)
	case string(core.WindowCyclePrevious), string(core.WindowCycleNext), string(core.WindowPrevious), string(core.WindowNext):
		e.pulseHaptic(32000, 22000, 75*time.Millisecond)
	case string(core.WindowCycleCommit):
		e.pulseHaptic(42000, 32000, 110*time.Millisecond)
	case string(core.VoiceTap):
		e.pulseHaptic(26000, 30000, 90*time.Millisecond)
	default:
		e.pulseHaptic(20000, 24000, 70*time.Millisecond)
	}
}

func (e *Engine) pulseHaptic(left, right uint16, duration time.Duration) {
	if !e.options.HapticsEnabled || e.options.HapticStrength <= 0 {
		return
	}
	scale := func(amount uint16) uint16 {
		value := math.Round(float64(amount) * e.options.HapticStrength)
		if value > 65535 {
			value = 65535
		}
		return uint16(value)
	}
	e.rumbleLeft, e.rumbleRight = scale(left), scale(right)
	e.rumbleUntil = e.clock.Now().Add(duration)
}

func (e *Engine) disconnect() error {
	if e.smoothScrollActive && e.smoothScroller != nil {
		_ = e.smoothScroller.ScrollSmooth(0, core.SmoothScrollEnded)
	}
	e.smoothScrollActive = false
	e.smoothScrollVelocity = 0
	releaseErr := e.releaseInputs()
	e.clearCompose("disconnect")
	if e.device != "" {
		_ = e.gamepad.Rumble(e.device, 0, 0)
	}
	e.device = ""
	e.previousButtons = 0
	e.previousLeftTrigger = false
	e.previousRightTrigger = false
	e.previousLeftStick = false
	e.previousRightStick = false
	e.exitComboStarted = time.Time{}
	e.exitComboRecorded = false
	e.rumbleLeft, e.rumbleRight = 0, 0
	e.rumbleSentLeft, e.rumbleSentRight = 0, 0
	e.rumbleUntil = time.Time{}
	return releaseErr
}

func (e *Engine) shutdown() error {
	return e.disconnect()
}
