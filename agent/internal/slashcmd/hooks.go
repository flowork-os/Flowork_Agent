// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package slashcmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SlashHook interface {
	Name() string
	Before(ctx context.Context, text string) error
	After(ctx context.Context, cmdName string, res Result, runErr error)
}

var (
	hooksMu sync.RWMutex
	hooks   []SlashHook
)

func RegisterHook(h SlashHook) {
	if h == nil {
		return
	}
	hooksMu.Lock()
	defer hooksMu.Unlock()
	for _, existing := range hooks {
		if existing.Name() == h.Name() {
			return
		}
	}
	hooks = append(hooks, h)
}

func ListHooks() []string {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	out := make([]string, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, h.Name())
	}
	return out
}

func DispatchWithHooks(ctx context.Context, text string) (Result, string, error) {
	hooksMu.RLock()
	chain := make([]SlashHook, len(hooks))
	copy(chain, hooks)
	hooksMu.RUnlock()

	for _, h := range chain {
		if err := h.Before(ctx, text); err != nil {

			recordBlockedDecision(ctx, text, h.Name(), err)
			return Result{}, "", fmt.Errorf("blocked by %s: %w", h.Name(), err)
		}
	}

	res, cmdName, runErr := Dispatch(ctx, text)

	for _, h := range chain {
		h.After(ctx, cmdName, res, runErr)
	}

	return res, cmdName, runErr
}

func recordBlockedDecision(ctx context.Context, text, hookName string, err error) {
	store, ok := FromStore(ctx)
	if !ok {
		return
	}
	caller := FromCaller(ctx)
	inputs := map[string]any{
		"text":   text,
		"hook":   hookName,
		"caller": caller,
	}
	_, _ = store.LogDecision(
		"slash_blocked",
		err.Error(),
		"fail",
		inputs,
		0,
	)
}

type decisionsLogHook struct{}

func (decisionsLogHook) Name() string { return "decisions-log" }

func (decisionsLogHook) Before(_ context.Context, _ string) error {
	return nil
}

func (decisionsLogHook) After(ctx context.Context, cmdName string, res Result, runErr error) {
	store, ok := FromStore(ctx)
	if !ok {
		return
	}
	caller := FromCaller(ctx)
	agentID := FromAgent(ctx)
	outcome := "success"
	rationale := "slash dispatched: /" + cmdName
	if runErr != nil {
		outcome = "fail"
		rationale = runErr.Error()
		if len(rationale) > 512 {
			rationale = rationale[:512] + "…"
		}
	} else if cmdName == "" {
		outcome = "skip"
		rationale = "no command matched"
	}
	preview := res.Text
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}
	inputs := map[string]any{
		"command":     cmdName,
		"agent_id":    agentID,
		"caller":      caller,
		"text_format": res.Format,
		"preview":     preview,
	}
	_, _ = store.LogDecision(
		"slash_dispatch",
		rationale,
		outcome,
		inputs,
		0,
	)
}

type rateLimitHook struct {
	mu     sync.Mutex
	window map[string][]time.Time
	cap    int
	period time.Duration
}

func (h *rateLimitHook) Name() string { return "rate-limit" }

func (h *rateLimitHook) Before(ctx context.Context, _ string) error {
	agentID := FromAgent(ctx)
	if agentID == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.window == nil {
		h.window = map[string][]time.Time{}
	}
	now := time.Now()
	cutoff := now.Add(-h.period)

	cur := h.window[agentID]
	next := cur[:0]
	for _, t := range cur {
		if t.After(cutoff) {
			next = append(next, t)
		}
	}
	if len(next) >= h.cap {
		h.window[agentID] = next
		return fmt.Errorf("rate limit: %d/%s exceeded", h.cap, h.period)
	}
	next = append(next, now)
	h.window[agentID] = next
	return nil
}

func (h *rateLimitHook) After(_ context.Context, _ string, _ Result, _ error) {}

func InitHooks() {
	RegisterHook(&rateLimitHook{cap: 30, period: 60 * time.Second})
	RegisterHook(decisionsLogHook{})
	_ = strings.ToLower
}
