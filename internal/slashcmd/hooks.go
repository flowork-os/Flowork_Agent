// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 17 phase 3 — pre/post hook framework. Locked Dispatch
//   ngga di-modify — caller pakai DispatchWithHooks instead. Phase 4
//   (priority ordering, async hooks, hook timeout) → tambah file baru.
//
// hooks.go — Section 17 phase 3: slash hook framework.
//
// PRE-HOOK semantik: Before invocation. Return non-nil err → block invoke
// + record decisions log dengan type="slash_blocked". Common use:
// rate-limit, capability check, malicious pattern detect.
//
// POST-HOOK semantik: After invocation. Receive result + err. Common use:
// decisions log append (this file built-in), karma update, audit trail.
//
// Built-in DecisionsLogHook auto-registers via Init() — caller (main)
// panggil InitDecisionsHook supaya tiap slash dispatch ke-record di
// decisions table (audit per Section 3 requirement).

package slashcmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SlashHook — interface buat hook chain.
type SlashHook interface {
	Name() string
	Before(ctx context.Context, text string) error
	After(ctx context.Context, cmdName string, res Result, runErr error)
}

var (
	hooksMu sync.RWMutex
	hooks   []SlashHook
)

// RegisterHook — append hook ke global chain. Idempotent name check.
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

// ListHooks snapshot.
func ListHooks() []string {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	out := make([]string, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, h.Name())
	}
	return out
}

// DispatchWithHooks — wrap Dispatch dengan hook chain. Caller (kernelhost
// SlashDispatcherFunc) panggil ini instead of Dispatch langsung untuk
// dapet full pipeline.
//
// Flow:
//
//   1. Before-hook chain (in registration order). Pertama yg err → block.
//      Record decisions log "slash_blocked" + return err.
//   2. Run Dispatch (existing locked).
//   3. After-hook chain (all called regardless of runErr).
func DispatchWithHooks(ctx context.Context, text string) (Result, string, error) {
	hooksMu.RLock()
	chain := make([]SlashHook, len(hooks))
	copy(chain, hooks)
	hooksMu.RUnlock()

	// Pre-hook chain.
	for _, h := range chain {
		if err := h.Before(ctx, text); err != nil {
			// Block: record + return.
			recordBlockedDecision(ctx, text, h.Name(), err)
			return Result{}, "", fmt.Errorf("blocked by %s: %w", h.Name(), err)
		}
	}

	// Dispatch.
	res, cmdName, runErr := Dispatch(ctx, text)

	// Post-hook chain (all called).
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

// =============================================================================
// Built-in: DecisionsLogHook — append decisions row after every dispatch
// =============================================================================

type decisionsLogHook struct{}

func (decisionsLogHook) Name() string { return "decisions-log" }

func (decisionsLogHook) Before(_ context.Context, _ string) error {
	return nil // no pre-action, audit happens After
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

// =============================================================================
// Built-in: RateLimitHook — block kalau >30 slash dispatch / 60s per agent
// =============================================================================

type rateLimitHook struct {
	mu     sync.Mutex
	window map[string][]time.Time // agent_id → recent timestamps
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
	// Filter window.
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

// =============================================================================
// InitHooks — register built-in hooks. Caller (main) panggil exactly once.
// =============================================================================

func InitHooks() {
	RegisterHook(&rateLimitHook{cap: 30, period: 60 * time.Second})
	RegisterHook(decisionsLogHook{})
	_ = strings.ToLower // anti-unused-import sentinel
}
