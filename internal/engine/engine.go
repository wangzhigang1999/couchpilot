package engine

import (
	"context"
	"errors"
	"io"
	"log"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

var ErrExitRequested = errors.New("emergency exit requested")

const (
	composeDeleteRepeatDelay    = 320 * time.Millisecond
	composeDeleteRepeatInterval = 75 * time.Millisecond
)

type Engine struct {
	options        Options
	gamepad        core.Gamepad
	desktop        core.Desktop
	smoothScroller core.SmoothScroller
	clock          core.Clock
	resolver       *mapping.Resolver
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
	composeForegroundApp string
	composeReadyAt       time.Time
	composeUntil         time.Time
	repeatButton         core.Button
	repeatAction         core.Action
	repeatNext           time.Time
	heldActions          map[core.Button]core.Operation
	scrollRemainder      float64
	smoothScrollVelocity float64
	smoothScrollActive   bool
	moveRemainder        [2]float64
	frameContextLoaded   bool
	frameProfile         string
	frameProcessName     string
}

func NewWithOptions(options Options, gamepad core.Gamepad, desktop core.Desktop, verbose bool, output io.Writer) *Engine {
	if output == nil {
		output = io.Discard
	}
	result := &Engine{
		options:     options,
		gamepad:     gamepad,
		desktop:     desktop,
		clock:       core.RealClock{},
		resolver:    mapping.NewResolver(options.Bindings),
		logger:      log.New(output, "", log.LstdFlags),
		verbose:     verbose,
		heldActions: make(map[core.Button]core.Operation),
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
	e.logger.Printf("emergency exit: hold Back + Start for %.1f seconds", e.options.ExitHoldSeconds)
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
		state, connected, err := e.gamepad.Read(e.device, e.options.Deadzone)
		if err != nil {
			return err
		}
		if !connected {
			e.logger.Printf("controller disconnected; waiting for reconnect")
			e.disconnect()
			e.clock.Sleep(500 * time.Millisecond)
			continue
		}
		if device, alternative, found, err := e.findAlternativeInput(state); err != nil {
			return err
		} else if found {
			previous := e.device
			e.disconnect()
			e.device = device
			e.logger.Printf("controller %s took over from %s", device, previous)
			e.pulseHaptic(28000, 20000, 120*time.Millisecond)
			e.updateRumble(frameStarted)
			state = alternative
		}
		if err := e.Step(state, dt, frameStarted); err != nil {
			return err
		}
		wait := time.Second/time.Duration(e.options.PollHz) - e.clock.Now().Sub(frameStarted)
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
		} else if !e.exitComboRecorded && now.Sub(e.exitComboStarted).Seconds() >= e.options.ExitHoldSeconds {
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
