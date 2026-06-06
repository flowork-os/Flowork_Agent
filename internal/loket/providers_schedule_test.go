package loket

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestParseCronAndMatch(t *testing.T) {
	cs, err := parseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	at := func(min int) time.Time { return time.Date(2026, 1, 1, 10, min, 0, 0, time.UTC) }
	if !cs.match(at(15)) {
		t.Error("*/15 should match :15")
	}
	if cs.match(at(7)) {
		t.Error("*/15 should not match :07")
	}

	cs2, _ := parseCron("30 9 * * *")
	if !cs2.match(time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)) {
		t.Error("30 9 should match 09:30")
	}
	if cs2.match(time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)) {
		t.Error("30 9 should not match 10:30")
	}

	if _, err := parseCron("bad"); err == nil {
		t.Error("non-5-field spec should error")
	}
	if _, err := parseCron("60 * * * *"); err == nil {
		t.Error("minute 60 out of range should error")
	}
}

// TestScheduleAfterFires — schedule.after delivers a {kind:"schedule"} message to
// the registering module's handle (via the bus) when the timer elapses.
func TestScheduleAfterFires(t *testing.T) {
	fired := make(chan Message, 1)
	sp := &scheduleProviders{deps: Deps{Send: func(_ context.Context, target string, msg Message) error {
		fired <- msg
		return nil
	}}}
	if _, err := sp.after(context.Background(), "m", json.RawMessage(`{"delay_ms":20,"type":"tick","payload":{"x":1}}`)); err != nil {
		t.Fatalf("after: %v", err)
	}
	select {
	case msg := <-fired:
		if msg.Source.Kind != "schedule" || msg.Type != "tick" {
			t.Errorf("wrong fired message: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("schedule.after did not fire within 2s")
	}

	if _, err := sp.after(context.Background(), "m", json.RawMessage(`{"delay_ms":0}`)); err == nil {
		t.Error("delay_ms<=0 should error")
	}
}
