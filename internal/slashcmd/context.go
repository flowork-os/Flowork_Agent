// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 15 phase 1 ctx propagation. Mirror tools/context.go
//   pattern. API stable: WithStore/FromStore, WithCaller/FromCaller,
//   WithAgent/FromAgent. ctxKey type private anti-collision.
//
// context.go — ctx propagation buat slash command runtime.

package slashcmd

import (
	"context"

	"flowork-gui/internal/agentdb"
)

type ctxKey int

const (
	keyStore  ctxKey = iota
	keyCaller
	keyAgent
)

// WithStore — attach per-agent *agentdb.Store ke ctx. Dispatcher (kernelhost
// atau agentmgr) panggil sebelum Dispatch.
func WithStore(ctx context.Context, s *agentdb.Store) context.Context {
	return context.WithValue(ctx, keyStore, s)
}

// FromStore — extract. nil + false kalau ngga ada (slash harus reject
// kalau butuh store).
func FromStore(ctx context.Context) (*agentdb.Store, bool) {
	s, ok := ctx.Value(keyStore).(*agentdb.Store)
	return s, ok && s != nil
}

// WithCaller — attach caller identity string (mis. 'telegram:<chat_id>',
// 'http-admin', 'rpc').
func WithCaller(ctx context.Context, caller string) context.Context {
	return context.WithValue(ctx, keyCaller, caller)
}

// FromCaller — default 'unknown' kalau ngga ada.
func FromCaller(ctx context.Context) string {
	c, _ := ctx.Value(keyCaller).(string)
	if c == "" {
		return "unknown"
	}
	return c
}

// WithAgent — attach agent ID.
func WithAgent(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, keyAgent, agentID)
}

// FromAgent — extract, default empty string.
func FromAgent(ctx context.Context) string {
	a, _ := ctx.Value(keyAgent).(string)
	return a
}
