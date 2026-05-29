// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 11 phase 1a (ctx propagation) DONE. API stable:
//   WithStore/FromStore, WithCaller/FromCaller, WithAgent/FromAgent.
//   ctxKey type private. Phase 2 extend (WithDeadline, WithCapability
//   set, WithSpan tracing) → tambah file baru, JANGAN modify ini.
//
// context.go — ctx propagation buat tool runtime.
//
// Tool.Run signature `(ctx, args)` ngga punya akses langsung ke agentdb
// store atau caller metadata. Dispatcher inject lewat ctx values yang
// tool extract via FromXxx helpers.

package tools

import (
	"context"

	"flowork-gui/internal/agentdb"
)

// ctxKey type private supaya ngga collide dengan ctx key dari package lain.
type ctxKey int

const (
	keyStore  ctxKey = iota // *agentdb.Store
	keyCaller               // string identifier (mis. 'daemon', 'rpc', 'http-admin')
	keyAgent                // string agent id (mr-flow)
)

// WithStore — attach per-agent *agentdb.Store ke ctx. Dipanggil dispatcher
// sebelum Run.
func WithStore(ctx context.Context, s *agentdb.Store) context.Context {
	return context.WithValue(ctx, keyStore, s)
}

// FromStore — extract store. Return nil + ok=false kalau ngga ada (tool
// harus handle gracefully).
func FromStore(ctx context.Context) (*agentdb.Store, bool) {
	s, ok := ctx.Value(keyStore).(*agentdb.Store)
	return s, ok && s != nil
}

// WithCaller — attach caller identity string (mis. 'daemon', 'http-admin',
// 'rpc', 'skill:<id>') ke ctx. Buat audit log.
func WithCaller(ctx context.Context, caller string) context.Context {
	return context.WithValue(ctx, keyCaller, caller)
}

// FromCaller — extract caller, default 'unknown'.
func FromCaller(ctx context.Context) string {
	c, _ := ctx.Value(keyCaller).(string)
	if c == "" {
		return "unknown"
	}
	return c
}

// WithAgent — attach agent ID. Buat tool-cross-agent logic future.
func WithAgent(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, keyAgent, agentID)
}

// FromAgent — extract agent ID, default empty string.
func FromAgent(ctx context.Context) string {
	a, _ := ctx.Value(keyAgent).(string)
	return a
}
