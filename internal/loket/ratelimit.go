package loket

import (
	"sync"
	"time"
)

// rateLimiter caps how many calls a single module may make per time window, so a
// buggy or runaway module cannot exhaust resources or spend money in a tight loop
// (e.g. calling llm.complete forever). It is a fixed-window counter — cheap and
// good enough for a safety cap; precision is not the goal, containment is.
//
// A nil limiter, or a limit <= 0, means "no limit" (the kernel default), so the
// mechanism is opt-in and never changes behaviour until the owner sets a cap.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	state  map[string]*rlEntry
}

type rlEntry struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &rateLimiter{limit: limit, window: window, state: map[string]*rlEntry{}}
}

// SetRateLimit caps each module to `callsPerMinute` calls. 0 disables the cap.
// Off by default so behaviour is unchanged until the owner opts in.
func (k *Kernel) SetRateLimit(callsPerMinute int) {
	if callsPerMinute <= 0 {
		k.limiter = nil
		return
	}
	k.limiter = newRateLimiter(callsPerMinute, time.Minute)
}

// allow reports whether module may make another call right now, and records it.
func (r *rateLimiter) allow(module string) bool {
	if r == nil || r.limit <= 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	e := r.state[module]
	if e == nil || now.Sub(e.start) >= r.window {
		r.state[module] = &rlEntry{start: now, count: 1}
		return true
	}
	if e.count >= r.limit {
		return false
	}
	e.count++
	return true
}
