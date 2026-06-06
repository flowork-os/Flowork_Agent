package loket

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// scheduleProviders back schedule.after (one-shot) + schedule.cron (recurring),
// §2.D of the contract. When a timer fires, the kernel delivers a
// {source.kind:"schedule"} message to the REGISTERING module's handle over the bus
// — so a module can wake itself later (watchers, periodic tasks) without owning a
// thread of its own. State is in-memory: a module re-establishes its schedules on
// its next load (on_load/boot), which keeps the kernel simple and stateless.
type scheduleProviders struct {
	deps    Deps
	mu      sync.Mutex
	crons   []cronJob
	started bool
}

type cronJob struct {
	module  string
	spec    cronSpec
	typ     string
	payload json.RawMessage
}

// after schedules a single delivery after delay_ms (capped at 7 days).
func (s *scheduleProviders) after(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		DelayMs int             `json:"delay_ms"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.DelayMs <= 0 {
		return nil, fmt.Errorf("schedule.after: delay_ms must be > 0")
	}
	if max := 7 * 24 * 3600 * 1000; a.DelayMs > max {
		a.DelayMs = max
	}
	typ, payload := a.Type, a.Payload
	time.AfterFunc(time.Duration(a.DelayMs)*time.Millisecond, func() {
		s.fire(module, typ, payload)
	})
	return mustJSON(map[string]any{"ok": true, "fires_in_ms": a.DelayMs}), nil
}

// cron registers a recurring delivery on a 5-field cron spec.
func (s *scheduleProviders) cron(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Spec    string          `json:"spec"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	spec, err := parseCron(a.Spec)
	if err != nil {
		return nil, fmt.Errorf("schedule.cron: %w", err)
	}
	s.mu.Lock()
	s.crons = append(s.crons, cronJob{module: module, spec: spec, typ: a.Type, payload: a.Payload})
	if !s.started {
		s.started = true
		go s.tickLoop()
	}
	s.mu.Unlock()
	return mustJSON(map[string]any{"ok": true, "spec": a.Spec}), nil
}

// fire delivers a schedule message to the module's handle over the bus.
func (s *scheduleProviders) fire(module, typ string, payload json.RawMessage) {
	if s.deps.Send == nil {
		return
	}
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	_ = s.deps.Send(ctx, module, Message{
		Source:  MsgSource{Kind: "schedule", ID: "kernel"},
		Type:    typ,
		Payload: payload,
	})
}

// tickLoop checks every cron job once a minute, aligned to the top of the minute.
func (s *scheduleProviders) tickLoop() {
	sleepToNextMinute := func() {
		time.Sleep(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)))
	}
	sleepToNextMinute()
	for {
		now := time.Now()
		s.mu.Lock()
		jobs := make([]cronJob, len(s.crons))
		copy(jobs, s.crons)
		s.mu.Unlock()
		for _, j := range jobs {
			if j.spec.match(now) {
				go s.fire(j.module, j.typ, j.payload)
			}
		}
		sleepToNextMinute()
	}
}

// ── minimal 5-field cron (minute hour day-of-month month day-of-week) ──────────

// cronSpec is a parsed cron. A nil field means "*" (matches anything); otherwise
// it is the set of allowed values for that field.
type cronSpec struct {
	min, hour, dom, mon, dow map[int]bool
}

func parseCron(spec string) (cronSpec, error) {
	f := strings.Fields(spec)
	if len(f) != 5 {
		return cronSpec{}, fmt.Errorf("want 5 fields, got %d", len(f))
	}
	var cs cronSpec
	var err error
	if cs.min, err = parseCronField(f[0], 0, 59); err != nil {
		return cs, err
	}
	if cs.hour, err = parseCronField(f[1], 0, 23); err != nil {
		return cs, err
	}
	if cs.dom, err = parseCronField(f[2], 1, 31); err != nil {
		return cs, err
	}
	if cs.mon, err = parseCronField(f[3], 1, 12); err != nil {
		return cs, err
	}
	if cs.dow, err = parseCronField(f[4], 0, 6); err != nil {
		return cs, err
	}
	return cs, nil
}

// parseCronField handles "*", "*/N", "a-b", "a,b", and exact values.
func parseCronField(field string, lo, hi int) (map[int]bool, error) {
	if field == "*" {
		return nil, nil
	}
	set := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		switch {
		case strings.HasPrefix(part, "*/"):
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("bad step %q", part)
			}
			for v := lo; v <= hi; v += step {
				set[v] = true
			}
		case strings.IndexByte(part, '-') > 0:
			i := strings.IndexByte(part, '-')
			a, e1 := strconv.Atoi(part[:i])
			b, e2 := strconv.Atoi(part[i+1:])
			if e1 != nil || e2 != nil || a < lo || b > hi || a > b {
				return nil, fmt.Errorf("bad range %q", part)
			}
			for v := a; v <= b; v++ {
				set[v] = true
			}
		default:
			v, err := strconv.Atoi(part)
			if err != nil || v < lo || v > hi {
				return nil, fmt.Errorf("bad value %q", part)
			}
			set[v] = true
		}
	}
	return set, nil
}

func (cs cronSpec) match(t time.Time) bool {
	hit := func(m map[int]bool, v int) bool { return m == nil || m[v] }
	return hit(cs.min, t.Minute()) && hit(cs.hour, t.Hour()) && hit(cs.dom, t.Day()) &&
		hit(cs.mon, int(t.Month())) && hit(cs.dow, int(t.Weekday()))
}
