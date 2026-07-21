package engine

import (
	"testing"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/trace"
)

type factSink struct {
	facts []trace.Fact
}

func (s *factSink) Record(fact trace.Fact) {
	s.facts = append(s.facts, fact)
}

func TestPhysicalGestureIncludesActiveTrigger(t *testing.T) {
	resolved := ResolvedBinding{Gesture: "rt+a"}
	if got := physicalGestureForAttempt("a", resolved, core.State{RightTrigger: 1}); got != "rt+a" {
		t.Fatalf("physical gesture = %q", got)
	}
}

func TestPhysicalGestureKeepsVoiceSequence(t *testing.T) {
	resolved := ResolvedBinding{Gesture: "voice+b"}
	if got := physicalGestureForAttempt("b", resolved, core.State{}); got != "voice+b" {
		t.Fatalf("physical gesture = %q", got)
	}
}

func TestEmitDefaultsOutcome(t *testing.T) {
	sink := &factSink{}
	engine := &Engine{traceSink: sink}
	engine.emit(trace.Fact{Kind: trace.InputAttempt, Control: "a"})
	if len(sink.facts) != 1 || sink.facts[0].Outcome != trace.NoOutcome {
		t.Fatalf("facts = %+v", sink.facts)
	}
}
