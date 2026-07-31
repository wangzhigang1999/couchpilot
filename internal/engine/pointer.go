package engine

import (
	"math"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

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
		multiplier = 1 - amount*(1-e.options.PrecisionSpeedMultiplier)
	} else if state.RightTrigger > 0.08 {
		amount := math.Min(1, (state.RightTrigger-0.08)/0.92)
		multiplier = 1 + amount*(e.options.BoostSpeedMultiplier-1)
	}
	speed := e.options.PointerMaxSpeed * math.Pow(magnitude, e.options.PointerCurve) * multiplier
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
	e.scrollRemainder += state.RightY * e.options.ScrollUnitsPerSecond * dt
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
	targetVelocity := axis * e.options.ScrollUnitsPerSecond
	response := 24.0
	if math.Abs(targetVelocity) < 1e-4 {
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
