// agentctx_ext.go — GROWTH-POINT (NON-frozen). Sibling buat authctx.go (LOCKED) —
// nambah identitas AGENT pemanggil di ctx TANPA buka file locked (Rule 7).
//
// #3 scoped-instinct (RI-5): agent kirim header `X-Agent-ID: <selfID>` pas call LLM
// (lihat agent fetch). Auth-middleware stash ke ctx via WithAgentID → selector insting
// (instinctenrich_ext2.go) baca AgentIDFromContext buat scope by-peran. Kosong = anonim
// (external/non-flowork atau agent belum di-rebuild) → fails-open (perilaku lama).
package router

import "context"

type ctxKeyAgentIDType struct{}

var ctxKeyAgentID = ctxKeyAgentIDType{}

// WithAgentID stashes the calling agent's id (from X-Agent-ID header) in ctx.
func WithAgentID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyAgentID, id)
}

// AgentIDFromContext returns the calling agent id, or "" for anonymous/external.
func AgentIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyAgentID).(string)
	return id
}
