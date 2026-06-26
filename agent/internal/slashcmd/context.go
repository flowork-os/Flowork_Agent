// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package slashcmd

import (
	"context"

	"flowork-gui/internal/agentdb"
)

type ctxKey int

const (
	keyStore ctxKey = iota
	keyCaller
	keyAgent
)

func WithStore(ctx context.Context, s *agentdb.Store) context.Context {
	return context.WithValue(ctx, keyStore, s)
}

func FromStore(ctx context.Context) (*agentdb.Store, bool) {
	s, ok := ctx.Value(keyStore).(*agentdb.Store)
	return s, ok && s != nil
}

func WithCaller(ctx context.Context, caller string) context.Context {
	return context.WithValue(ctx, keyCaller, caller)
}

func FromCaller(ctx context.Context) string {
	c, _ := ctx.Value(keyCaller).(string)
	if c == "" {
		return "unknown"
	}
	return c
}

func WithAgent(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, keyAgent, agentID)
}

func FromAgent(ctx context.Context) string {
	a, _ := ctx.Value(keyAgent).(string)
	return a
}
