package main

import (
	"sync/atomic"
	"testing"

	"flowork-gui/internal/floworkdb"
)

func TestWakeMandorRuleIDs(t *testing.T) {
	rules := []floworkdb.Trigger{
		{ID: "r1", TypeID: "worklog-pending", Enabled: true, Target: "mandor-group"},
		{ID: "r2", TypeID: "worklog-pending", Enabled: false, Target: "mandor-group"}, // disabled
		{ID: "r3", TypeID: "time", Enabled: true, Target: "mandor-group"},             // bukan worklog
		{ID: "r4", TypeID: "worklog-pending", Enabled: true, Target: "worker-a"},      // self-loop
	}
	got := wakeMandorRuleIDs(rules, "Worker-A")
	if len(got) != 1 || got[0] != "r1" {
		t.Fatalf("mau [r1] (enabled, worklog-pending, bukan self), dapet %v", got)
	}
}

func TestWakeMandorDebounce(t *testing.T) {
	atomic.StoreInt64(&wakeMandorLastMs, 0)
	if !wakeMandorDue(1_000_000) {
		t.Fatal("wake pertama harus lolos")
	}
	if wakeMandorDue(1_000_000 + wakeMandorDebounceMs - 1) {
		t.Fatal("dalam jendela debounce harus ke-block")
	}
	if !wakeMandorDue(1_000_000 + wakeMandorDebounceMs + 1) {
		t.Fatal("lewat jendela debounce harus lolos lagi")
	}
	atomic.StoreInt64(&wakeMandorLastMs, 0)
}
