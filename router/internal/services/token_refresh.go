// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package services

import (
	"context"
	"log"
	"sync"
	"time"
)

var RefreshLead = 5 * time.Minute

var RefreshLeadByProvider = map[string]time.Duration{
	"codex":       5 * 24 * time.Hour,
	"openai":      5 * 24 * time.Hour,
	"claude":      4 * time.Hour,
	"anthropic":   4 * time.Hour,
	"iflow":       24 * time.Hour,
	"qwen":        20 * time.Minute,
	"kimi-coding": 5 * time.Minute,
	"kimi":        5 * time.Minute,
	"antigravity": 5 * time.Minute,
	"gemini-cli":  5 * time.Minute,
	"github":      4 * time.Hour,
	"copilot":     4 * time.Hour,
	"kiro":        4 * time.Hour,
}

func leadFor(provider string) time.Duration {
	if d, ok := RefreshLeadByProvider[provider]; ok && d > 0 {
		return d
	}
	return RefreshLead
}

var FailureRetry = 60 * time.Second

type TokenSource interface {
	Provider() string
	ExpiresAt() time.Time
	Refresh(ctx context.Context) (time.Time, error)
}

type Worker struct {
	mu      sync.Mutex
	sources []TokenSource
	cancel  context.CancelFunc
	started bool
	wg      sync.WaitGroup
}

func NewWorker() *Worker { return &Worker{} }

func (w *Worker) Add(src TokenSource) {
	w.mu.Lock()
	w.sources = append(w.sources, src)
	w.mu.Unlock()
}

func (w *Worker) Start() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.started = true
	w.wg.Add(1)
	w.mu.Unlock()
	go func() {
		defer w.wg.Done()
		w.loop(ctx)
	}()
}

func (w *Worker) Stop() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	w.started = false
	w.mu.Unlock()
	w.wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	for {
		w.mu.Lock()
		sources := append([]TokenSource(nil), w.sources...)
		w.mu.Unlock()

		now := time.Now()
		var nextWake time.Duration = FailureRetry
		var due TokenSource
		for _, s := range sources {
			exp := s.ExpiresAt()
			if exp.IsZero() {
				continue
			}
			refreshAt := exp.Add(-leadFor(s.Provider()))
			if !refreshAt.After(now) {

				due = s
				nextWake = 0
				break
			}
			wait := time.Until(refreshAt)
			if wait < nextWake {
				nextWake = wait
			}
		}

		if due != nil {
			if newExp, err := due.Refresh(ctx); err != nil {
				log.Printf("flow_router token refresh failed for %s: %v", due.Provider(), err)
				nextWake = FailureRetry
			} else {
				log.Printf("flow_router token refreshed for %s; next expiry %s", due.Provider(), newExp.Format(time.RFC3339))
				continue
			}
		}

		if nextWake <= 0 {
			nextWake = time.Hour
		}
		if nextWake > time.Hour {
			nextWake = time.Hour
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(nextWake):
		}
	}
}
