package engine

import (
	"math/bits"
	"sort"
	"strings"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

func (e *Engine) emit(fact trace.Fact) {
	if e.traceSink == nil {
		return
	}
	if fact.Outcome == "" {
		fact.Outcome = trace.NoOutcome
	}
	e.traceSink.Record(fact)
}

func physicalGestureForAttempt(control string, resolved ResolvedBinding, state core.State) string {
	if strings.HasPrefix(resolved.Gesture, "voice+") {
		return resolved.Gesture
	}
	parts := make([]string, 0, 3)
	if state.LeftTrigger > 0.08 {
		parts = append(parts, "lt")
	}
	if state.RightTrigger > 0.08 {
		parts = append(parts, "rt")
	}
	parts = append(parts, control)
	return strings.Join(parts, "+")
}

func pressedButtonCount(pressed core.Button) int {
	return bits.OnesCount16(uint16(pressed))
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

func flagIf(condition bool, value string) string {
	if condition {
		return value
	}
	return ""
}
