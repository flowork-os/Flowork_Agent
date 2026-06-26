// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package router

import "context"

type ctxKeyAgentIDType struct{}

var ctxKeyAgentID = ctxKeyAgentIDType{}

func WithAgentID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyAgentID, id)
}

func AgentIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyAgentID).(string)
	return id
}
