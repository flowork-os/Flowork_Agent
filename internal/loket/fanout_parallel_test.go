package loket

import (
	"encoding/json"
	"testing"
	"time"
)

// TestParallelFanout_All covers the happy path: every member finishes, all replies
// collected in input order.
func TestParallelFanout_All(t *testing.T) {
	targets := []string{"a", "b", "c"}
	invoke := func(target string) (json.RawMessage, error) {
		return json.RawMessage(`"` + target + `"`), nil
	}
	out := ParallelFanout(time.Second, targets, invoke)
	if len(out) != 3 {
		t.Fatalf("want 3 replies, got %d", len(out))
	}
	for i, tg := range targets {
		if out[i].Target != tg || out[i].Error != "" {
			t.Fatalf("reply %d: %+v", i, out[i])
		}
	}
}

// TestParallelFanout_Bounded is the coordination lifecycle: a member that hangs past
// the budget is reported as a timeout while the fast members still return — the council
// is never held hostage by one stuck member.
func TestParallelFanout_Bounded(t *testing.T) {
	stuck := make(chan struct{})
	defer close(stuck) // unblock the hung goroutine at test end
	invoke := func(target string) (json.RawMessage, error) {
		if target == "slow" {
			<-stuck // blocks well past the budget
		}
		return json.RawMessage(`"` + target + `"`), nil
	}
	out := ParallelFanout(80*time.Millisecond, []string{"fast", "slow"}, invoke)
	if len(out) != 2 {
		t.Fatalf("want 2 replies, got %d", len(out))
	}
	if out[0].Target != "fast" || out[0].Error != "" {
		t.Fatalf("fast member should complete: %+v", out[0])
	}
	if out[1].Error == "" {
		t.Fatalf("slow member should be a budget-exceeded timeout, got %+v", out[1])
	}
}
