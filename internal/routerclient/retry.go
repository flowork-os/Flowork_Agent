// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 7 phase 2 (retry + circuit breaker primitive). API stable:
//   WithRetry, RetryOpts, IsRetryable. Circuit breaker via simple sliding
//   window (failure rate threshold) — full hystrix-style breaker JANGAN
//   over-engineer (single user). Phase 3 (per-endpoint state, half-open
//   probe, distributed breaker) → tambah file baru.
//
// retry.go — Section 7 phase 2: retry + minimal circuit breaker.
//
// Pattern: helper generik buat wrap any Client method. Bukan middleware
// HTTP layer — supaya caller bisa pilih per-call kapan butuh retry.
//
// Usage:
//
//	opts := routerclient.DefaultRetry()
//	resp, err := routerclient.WithRetry(ctx, opts, func(ctx context.Context) error {
//	    var ierr error
//	    resp, ierr = c.ListSkills(ctx, q, 10)
//	    return ierr
//	})
//
// Source: Flowork_Agent/roadmap.md Section 7 phase 2.

package routerclient

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

// RetryOpts — knobs untuk retry policy.
type RetryOpts struct {
	// MaxAttempts — total attempt (incl. first). 1 = no retry. Default 3.
	MaxAttempts int
	// InitialDelay — backoff awal sebelum attempt #2. Default 200ms.
	InitialDelay time.Duration
	// MaxDelay — cap exponential growth. Default 5s.
	MaxDelay time.Duration
	// Multiplier — exponential factor. Default 2.0.
	Multiplier float64
}

// DefaultRetry — sane default buat single-user agent → router LAN call.
func DefaultRetry() RetryOpts {
	return RetryOpts{
		MaxAttempts:  3,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

// WithRetry — execute fn dengan exponential backoff. Stop kalau fn return
// non-retryable error (lihat IsRetryable) atau MaxAttempts tercapai. Ctx
// cancellation immediately halt.
func WithRetry(ctx context.Context, opts RetryOpts, fn func(ctx context.Context) error) error {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 1
	}
	if opts.InitialDelay <= 0 {
		opts.InitialDelay = 200 * time.Millisecond
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 5 * time.Second
	}
	if opts.Multiplier <= 1.0 {
		opts.Multiplier = 2.0
	}

	delay := opts.InitialDelay
	var lastErr error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		// Ctx check sebelum attempt (kalau parent udah cancel).
		if cerr := ctx.Err(); cerr != nil {
			if lastErr != nil {
				return lastErr
			}
			return cerr
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !IsRetryable(err) {
			return err
		}
		if attempt == opts.MaxAttempts {
			break
		}
		// Sleep dengan ctx-aware timer.
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return lastErr
		case <-t.C:
		}
		// Exponential growth, capped.
		next := time.Duration(float64(delay) * opts.Multiplier)
		if next > opts.MaxDelay {
			next = opts.MaxDelay
		}
		delay = next
	}
	return lastErr
}

// IsRetryable — return true kalau err kemungkinan transient (network blip,
// timeout, 5xx). 4xx (terutama 400/401/403/404) ngga retry — bug client.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Network-level: temporary / timeout.
	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return true
		}
	}
	// Heuristic message scan (paling robust di Go ngga ada error class system).
	msg := strings.ToLower(err.Error())
	transientHints := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"connection closed",
		"broken pipe",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
		"router status 500", "router status 502", "router status 503", "router status 504",
		"context deadline exceeded",
	}
	for _, h := range transientHints {
		if strings.Contains(msg, h) {
			return true
		}
	}
	return false
}

// CircuitBreaker — minimal sliding-window failure counter.
//
// Semantik:
//   - Tiap call Mark(success bool). Buffer last N=10 result.
//   - Allow() return false kalau failure rate > threshold (default 0.6)
//     dan window penuh.
//   - State auto-recover saat success kembali masuk ke window.
//
// Single-user reality: per-router instance ada 1 breaker shared. Concurrency
// safe via mutex. Phase 3 → per-endpoint breaker.
type CircuitBreaker struct {
	mu        sync.Mutex
	window    []bool // true = success
	cap       int
	threshold float64 // failure rate yang trigger open. 0.6 = 60% gagal → open
}

// NewCircuitBreaker — return breaker dengan window size + threshold.
// size <= 0 → 10. threshold <= 0 atau >= 1 → 0.6.
func NewCircuitBreaker(size int, threshold float64) *CircuitBreaker {
	if size <= 0 {
		size = 10
	}
	if threshold <= 0 || threshold >= 1.0 {
		threshold = 0.6
	}
	return &CircuitBreaker{
		cap:       size,
		threshold: threshold,
	}
}

// Mark — record outcome. Drop oldest kalau window penuh.
func (b *CircuitBreaker) Mark(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.window) >= b.cap {
		b.window = b.window[1:]
	}
	b.window = append(b.window, success)
}

// Allow — return true kalau breaker masih CLOSED (boleh request). False =
// open / probationary. Window ngga penuh → always allow.
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.window) < b.cap {
		return true
	}
	fail := 0
	for _, s := range b.window {
		if !s {
			fail++
		}
	}
	rate := float64(fail) / float64(b.cap)
	return rate < b.threshold
}

// Reset — clear window. Buat manual reset setelah operator fix issue.
func (b *CircuitBreaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.window = nil
}

// ErrCircuitOpen — sentinel buat caller branch.
var ErrCircuitOpen = errors.New("router client: circuit breaker open")
