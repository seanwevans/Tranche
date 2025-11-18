package monitor

import (
	"context"
	"testing"
)

func TestSchedulerPreserveExistingLoops(t *testing.T) {
	s := &Scheduler{
		loops: make(map[string]context.CancelFunc),
	}
	s.loops["1:10:a"] = func() {}
	s.loops["1:11:b"] = func() {}
	s.loops["2:10:other"] = func() {}

	active := make(map[string]struct{})
	s.preserveExistingLoops(active, 1)

	if len(active) != 2 {
		t.Fatalf("expected 2 active loops, got %d", len(active))
	}

	if _, ok := active["1:10:a"]; !ok {
		t.Fatalf("expected loop 1:10:a to be preserved")
	}
	if _, ok := active["1:11:b"]; !ok {
		t.Fatalf("expected loop 1:11:b to be preserved")
	}
	if _, ok := active["2:10:other"]; ok {
		t.Fatalf("did not expect loop 2:10:other to be preserved")
	}
}
