// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package routerclient

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

type RetryOpts struct {
	MaxAttempts int

	InitialDelay time.Duration

	MaxDelay time.Duration

	Multiplier float64
}

func DefaultRetry() RetryOpts {
	return RetryOpts{
		MaxAttempts:  3,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

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

		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return lastErr
		case <-t.C:
		}

		next := time.Duration(float64(delay) * opts.Multiplier)
		if next > opts.MaxDelay {
			next = opts.MaxDelay
		}
		delay = next
	}
	return lastErr
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return true
		}
	}

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

type CircuitBreaker struct {
	mu        sync.Mutex
	window    []bool
	cap       int
	threshold float64
}

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

func (b *CircuitBreaker) Mark(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.window) >= b.cap {
		b.window = b.window[1:]
	}
	b.window = append(b.window, success)
}

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

func (b *CircuitBreaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.window = nil
}

var ErrCircuitOpen = errors.New("router client: circuit breaker open")
