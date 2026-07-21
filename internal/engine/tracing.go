package engine

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/config"
	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/usage"
)

const traceSemanticsVersion = "2"

type buttonTrace struct {
	started         time.Time
	foregroundApp   string
	activeProfile   string
	physicalGesture string
	gesture         string
	action          string
}

type triggerTrace struct {
	started       time.Time
	foregroundApp string
	activeProfile string
	pointerUsed   bool
	chordProbes   int
}

type composeTrace struct {
	active        bool
	started       time.Time
	foregroundApp string
	activeProfile string
	deletes       int
	repeats       int
}

type repeatTrace struct {
	active          bool
	started         time.Time
	foregroundApp   string
	activeProfile   string
	physicalGesture string
	gesture         string
	action          string
	count           int
}

type windowTrace struct {
	active        bool
	started       time.Time
	foregroundApp string
	activeProfile string
	previous      int
	next          int
}

type traceState struct {
	buttons      map[core.Button]buttonTrace
	leftTrigger  triggerTrace
	rightTrigger triggerTrace
	compose      composeTrace
	repeat       repeatTrace
	window       windowTrace
	lastAt       time.Time
}

func newTraceState() traceState {
	return traceState{buttons: make(map[core.Button]buttonTrace)}
}

func strategyID(settings config.Settings, inventory []usage.BindingDefinition) string {
	return strategyIDForVersion(settings, inventory, traceSemanticsVersion)
}

func strategyIDForVersion(settings config.Settings, inventory []usage.BindingDefinition, semanticsVersion string) string {
	routes, _ := json.Marshal(settings.AppProfiles)
	return usage.StrategyID(inventory, map[string]string{
		"app_profiles":                 string(routes),
		"deadzone":                     strconv.FormatFloat(settings.Deadzone, 'g', -1, 64),
		"poll_hz":                      strconv.Itoa(settings.PollHz),
		"trace_semantics_version":      semanticsVersion,
		"voice_mode":                   settings.VoiceMode,
		"voice_submit_timeout_seconds": strconv.FormatFloat(settings.VoiceSubmitTimeoutSeconds, 'g', -1, 64),
	})
}

func durationBucket(value time.Duration) string {
	switch {
	case value < 100*time.Millisecond:
		return "lt_100ms"
	case value < 300*time.Millisecond:
		return "100_299ms"
	case value < time.Second:
		return "300_999ms"
	case value < 3*time.Second:
		return "1_2s"
	case value < 10*time.Second:
		return "3_9s"
	default:
		return "gte_10s"
	}
}

func intervalBucket(value time.Duration) string {
	switch {
	case value <= 0:
		return "same_frame"
	case value < 50*time.Millisecond:
		return "lt_50ms"
	case value < 150*time.Millisecond:
		return "50_149ms"
	case value < 400*time.Millisecond:
		return "150_399ms"
	case value < time.Second:
		return "400_999ms"
	case value < 3*time.Second:
		return "1_2s"
	default:
		return "gte_3s"
	}
}

func countBucket(value int) string {
	switch {
	case value <= 0:
		return "0"
	case value == 1:
		return "1"
	case value == 2:
		return "2"
	case value <= 4:
		return "3_4"
	default:
		return "gte_5"
	}
}

func stableFlags(values ...string) string {
	filtered := values[:0]
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	sort.Strings(filtered)
	return strings.Join(filtered, ",")
}

func gestureKind(gesture string) usage.GestureKind {
	switch {
	case strings.HasPrefix(gesture, "lt+") || strings.HasPrefix(gesture, "rt+"):
		return usage.GestureTriggerChord
	case strings.HasPrefix(gesture, "voice+"):
		return usage.GestureModeSequence
	case gesture == "back+start":
		return usage.GestureSystemHold
	default:
		return usage.GestureSingle
	}
}

func buttonControl(button core.Button) (string, bool) {
	for _, item := range gestures {
		if item.button == button {
			return item.gesture, true
		}
	}
	for _, item := range unboundSystemButtons {
		if item.button == button {
			return item.control, true
		}
	}
	return "", false
}

func (e *Engine) emit(observation usage.Observation) {
	if e.usage == nil {
		return
	}
	if observation.StrategyID == "" {
		observation.StrategyID = e.strategyID
	}
	if observation.Outcome == "" {
		observation.Outcome = usage.OutcomeNone
	}
	e.usage.Record(observation)
}

func (e *Engine) startButtonHolds(pressed core.Button, now time.Time, profile, foregroundApp string) {
	for _, button := range supportedButtons() {
		if pressed&button == 0 {
			continue
		}
		control, _ := buttonControl(button)
		e.trace.buttons[button] = buttonTrace{
			started:         now,
			foregroundApp:   foregroundApp,
			activeProfile:   profile,
			physicalGesture: control,
			gesture:         control,
		}
	}
}

func (e *Engine) finishButtonHolds(released core.Button, now time.Time, reason string) {
	for _, button := range supportedButtons() {
		if released&button == 0 {
			continue
		}
		e.finishButtonHold(button, now, reason)
	}
}

func (e *Engine) finishButtonHold(button core.Button, now time.Time, reason string) {
	hold, found := e.trace.buttons[button]
	if !found {
		return
	}
	delete(e.trace.buttons, button)
	e.emit(usage.Observation{
		At:              now,
		Kind:            usage.EventHoldEpisode,
		ForegroundApp:   hold.foregroundApp,
		ActiveProfile:   hold.activeProfile,
		Control:         hold.physicalGesture,
		PhysicalGesture: hold.physicalGesture,
		Gesture:         hold.gesture,
		GestureKind:     usage.GestureDigitalHold,
		RelatedAction:   hold.action,
		Resolution:      usage.ResolutionObserved,
		DurationBucket:  durationBucket(now.Sub(hold.started)),
		Reason:          reason,
	})
}

func supportedButtons() []core.Button {
	result := make([]core.Button, 0, len(gestures)+len(unboundSystemButtons))
	for _, item := range gestures {
		result = append(result, item.button)
	}
	for _, item := range unboundSystemButtons {
		result = append(result, item.button)
	}
	return result
}

func pressedButtonCount(pressed core.Button) int {
	count := 0
	for _, button := range supportedButtons() {
		if pressed&button != 0 {
			count++
		}
	}
	return count
}

func (e *Engine) recordChordProbes(
	now time.Time,
	physicalGesture string,
	effective ResolvedBinding,
	foregroundApp string,
	state core.State,
	leftActive, rightActive bool,
	leftCandidate, rightCandidate ResolvedBinding,
	selectedModifier string,
) {
	dual := leftActive && rightActive
	record := func(modifier string, candidate ResolvedBinding, active bool) {
		if !active {
			return
		}
		e.markChordProbe(modifier)
		started := e.trace.leftTrigger.started
		if modifier == "rt" {
			started = e.trace.rightTrigger.started
		}
		lead := time.Duration(0)
		if !started.IsZero() {
			lead = now.Sub(started)
		}
		selected := selectedModifier == modifier
		priorityBlocked := modifier == "rt" && leftActive
		fallback := selectedModifier == ""
		pointerContext := e.triggerPointerContext(modifier, state)
		e.emit(usage.Observation{
			At:                  now,
			Kind:                usage.EventChordProbe,
			ForegroundApp:       foregroundApp,
			ActiveProfile:       candidate.ActiveProfile,
			BindingProfile:      candidate.BindingProfile,
			Control:             physicalGesture,
			PhysicalGesture:     candidate.Gesture,
			Gesture:             candidate.Gesture,
			GestureKind:         usage.GestureTriggerChord,
			Action:              string(candidate.Action),
			RelatedGesture:      effective.Gesture,
			RelatedAction:       string(effective.Action),
			Resolution:          usage.ResolutionObserved,
			CandidateResolution: candidate.Resolution,
			IntervalBucket:      intervalBucket(lead),
			Flags: stableFlags(
				flagIf(selected, "selected"),
				flagIf(fallback, "fallback"),
				flagIf(dual, "dual_trigger"),
				flagIf(priorityBlocked, "priority_blocked"),
				flagIf(pointerContext, "pointer_context"),
				flagIf(mathHypotActive(state.LeftX, state.LeftY), "left_stick_active"),
				flagIf(mathHypotActive(state.RightX, state.RightY), "right_stick_active"),
			),
		})
	}
	record("lt", leftCandidate, leftActive)
	record("rt", rightCandidate, rightActive)
}

func mathHypotActive(x, y float64) bool {
	return x*x+y*y >= 1e-8
}

func (e *Engine) recordLateModifierProbes(modifier string, state core.State, now time.Time, profile, foregroundApp string) {
	heldBeforeFrame := core.Button(uint16(state.Buttons) & uint16(e.previousButtons))
	for _, button := range supportedButtons() {
		if heldBeforeFrame&button == 0 {
			continue
		}
		control, found := buttonControl(button)
		if !found {
			continue
		}
		hold, tracked := e.trace.buttons[button]
		if !tracked {
			continue
		}
		candidate := e.resolver.ResolveDetailed(profile, modifier+"+"+control)
		e.markChordProbe(modifier)
		pointerContext := e.triggerPointerContext(modifier, state)
		e.emit(usage.Observation{
			At:                  now,
			Kind:                usage.EventChordProbe,
			ForegroundApp:       foregroundApp,
			ActiveProfile:       candidate.ActiveProfile,
			BindingProfile:      candidate.BindingProfile,
			Control:             control,
			PhysicalGesture:     physicalGestureForAttempt(control, candidate, state),
			Gesture:             candidate.Gesture,
			GestureKind:         usage.GestureTriggerChord,
			Action:              string(candidate.Action),
			RelatedGesture:      hold.gesture,
			RelatedAction:       hold.action,
			Resolution:          usage.ResolutionObserved,
			CandidateResolution: candidate.Resolution,
			IntervalBucket:      intervalBucket(now.Sub(hold.started)),
			Flags: stableFlags(
				"late_modifier",
				flagIf(state.LeftTrigger > 0.08 && state.RightTrigger > 0.08, "dual_trigger"),
				flagIf(modifier == "rt" && state.LeftTrigger > 0.08, "priority_blocked"),
				flagIf(pointerContext, "pointer_context"),
				flagIf(mathHypotActive(state.LeftX, state.LeftY), "left_stick_active"),
				flagIf(mathHypotActive(state.RightX, state.RightY), "right_stick_active"),
			),
		})
	}
}

func (e *Engine) triggerPointerContext(modifier string, state core.State) bool {
	pointerUsed := e.trace.leftTrigger.pointerUsed
	if modifier == "rt" {
		pointerUsed = e.trace.rightTrigger.pointerUsed
	}
	return pointerUsed || mathHypotActive(state.LeftX, state.LeftY) || mathHypotActive(state.RightX, state.RightY)
}

func physicalGestureForAttempt(control string, resolved ResolvedBinding, state core.State) string {
	if strings.HasPrefix(resolved.Gesture, "voice+") {
		return resolved.Gesture
	}
	parts := make([]string, 0, 5)
	if state.LeftTrigger > 0.08 {
		parts = append(parts, "lt")
	}
	if state.RightTrigger > 0.08 {
		parts = append(parts, "rt")
	}
	var digital []string
	for _, button := range supportedButtons() {
		if state.Buttons&button == 0 {
			continue
		}
		if value, found := buttonControl(button); found {
			digital = append(digital, value)
		}
	}
	if len(digital) == 0 {
		digital = append(digital, control)
	}
	sort.Strings(digital)
	parts = append(parts, digital...)
	return strings.Join(parts, "+")
}

func (e *Engine) updateButtonHold(button core.Button, resolved ResolvedBinding) {
	hold, found := e.trace.buttons[button]
	if !found {
		return
	}
	hold.gesture = resolved.Gesture
	hold.action = string(resolved.Action)
	e.trace.buttons[button] = hold
}

func (e *Engine) startTrigger(control string, now time.Time, profile, foregroundApp string) {
	value := triggerTrace{started: now, activeProfile: profile, foregroundApp: foregroundApp}
	if control == "lt" {
		e.trace.leftTrigger = value
	} else {
		e.trace.rightTrigger = value
	}
}

func (e *Engine) markPointerModifier(control string) {
	if control == "lt" && !e.trace.leftTrigger.started.IsZero() {
		e.trace.leftTrigger.pointerUsed = true
	}
	if control == "rt" && !e.trace.rightTrigger.started.IsZero() {
		e.trace.rightTrigger.pointerUsed = true
	}
}

func (e *Engine) markChordProbe(control string) {
	if control == "lt" && !e.trace.leftTrigger.started.IsZero() {
		e.trace.leftTrigger.chordProbes++
	}
	if control == "rt" && !e.trace.rightTrigger.started.IsZero() {
		e.trace.rightTrigger.chordProbes++
	}
}

func (e *Engine) finishTrigger(control string, now time.Time, reason string) {
	value := &e.trace.leftTrigger
	if control == "rt" {
		value = &e.trace.rightTrigger
	}
	if value.started.IsZero() {
		return
	}
	role := "idle"
	switch {
	case value.pointerUsed && value.chordProbes > 0:
		role = "pointer_and_candidate"
	case value.pointerUsed:
		role = "pointer_only"
	case value.chordProbes > 0:
		role = "candidate_only"
	}
	e.emit(usage.Observation{
		At:              now,
		Kind:            usage.EventHoldEpisode,
		ForegroundApp:   value.foregroundApp,
		ActiveProfile:   value.activeProfile,
		Control:         control,
		PhysicalGesture: control,
		Gesture:         control,
		GestureKind:     usage.GestureTriggerHold,
		Resolution:      usage.ResolutionObserved,
		DurationBucket:  durationBucket(now.Sub(value.started)),
		CountBucket:     countBucket(value.chordProbes),
		Reason:          reason,
		Flags:           role,
	})
	*value = triggerTrace{}
}

func flagIf(condition bool, value string) string {
	if condition {
		return value
	}
	return ""
}

func (e *Engine) finishComposeTrace(now time.Time, reason string, outcome usage.Outcome) {
	value := e.trace.compose
	if !value.active {
		return
	}
	e.emit(usage.Observation{
		At:             now,
		Kind:           usage.EventComposeSession,
		ForegroundApp:  value.foregroundApp,
		ActiveProfile:  value.activeProfile,
		Gesture:        "voice",
		GestureKind:    usage.GestureModeSequence,
		Action:         string(core.Voice),
		RelatedAction:  string(core.Enter),
		Resolution:     usage.ResolutionObserved,
		Outcome:        outcome,
		DurationBucket: durationBucket(now.Sub(value.started)),
		CountBucket:    countBucket(value.deletes + value.repeats),
		Reason:         reason,
		Flags: stableFlags(
			"deletes_"+countBucket(value.deletes),
			"repeats_"+countBucket(value.repeats),
		),
	})
	e.trace.compose = composeTrace{}
}

func (e *Engine) startWindowTrace(now time.Time, profile, foregroundApp string, action core.Action) {
	if !e.trace.window.active {
		e.trace.window = windowTrace{active: true, started: now, activeProfile: profile, foregroundApp: foregroundApp}
	}
	if action == core.WindowPrevious {
		e.trace.window.previous++
	} else if action == core.WindowNext {
		e.trace.window.next++
	}
}

func (e *Engine) finishWindowTrace(now time.Time, reason string, outcome usage.Outcome) {
	value := e.trace.window
	if !value.active {
		return
	}
	gesture := "lt+lb"
	direction := "previous_only"
	if value.previous == 0 {
		gesture = "lt+rb"
		direction = "next_only"
	} else if value.next > 0 {
		gesture = "lt+lb|lt+rb"
		direction = "mixed"
	}
	e.emit(usage.Observation{
		At:             now,
		Kind:           usage.EventWindowSession,
		ForegroundApp:  value.foregroundApp,
		ActiveProfile:  value.activeProfile,
		Gesture:        gesture,
		GestureKind:    usage.GestureTriggerChord,
		Action:         "window_cycle",
		Resolution:     usage.ResolutionObserved,
		Outcome:        outcome,
		DurationBucket: durationBucket(now.Sub(value.started)),
		CountBucket:    countBucket(value.previous + value.next),
		Reason:         reason,
		Flags: stableFlags(
			direction,
			"next_"+countBucket(value.next),
			"previous_"+countBucket(value.previous),
		),
	})
	e.trace.window = windowTrace{}
}

func (e *Engine) startRepeatTrace(now time.Time, profile, foregroundApp, physicalGesture, gesture string, action core.Action) {
	e.finishRepeatTrace(now, "restarted", usage.OutcomeNone)
	e.trace.repeat = repeatTrace{
		active:          true,
		started:         now,
		foregroundApp:   foregroundApp,
		activeProfile:   profile,
		physicalGesture: physicalGesture,
		gesture:         gesture,
		action:          string(action),
	}
}

func (e *Engine) finishRepeatTrace(now time.Time, reason string, outcome usage.Outcome) {
	value := e.trace.repeat
	if !value.active {
		return
	}
	if value.count > 0 || outcome == usage.OutcomeFailure {
		e.emit(usage.Observation{
			At:              now,
			Kind:            usage.EventRepeatEpisode,
			ForegroundApp:   value.foregroundApp,
			ActiveProfile:   value.activeProfile,
			Control:         value.physicalGesture,
			PhysicalGesture: value.physicalGesture,
			Gesture:         value.gesture,
			GestureKind:     usage.GestureModeSequence,
			Action:          value.action,
			Resolution:      usage.ResolutionObserved,
			Outcome:         outcome,
			DurationBucket:  durationBucket(now.Sub(value.started)),
			CountBucket:     countBucket(value.count),
			Reason:          reason,
		})
	}
	e.trace.repeat = repeatTrace{}
}

func (e *Engine) traceDisconnect(now time.Time) {
	for button := range e.trace.buttons {
		e.finishButtonHold(button, now, "disconnect")
	}
	e.finishTrigger("lt", now, "disconnect")
	e.finishTrigger("rt", now, "disconnect")
}
