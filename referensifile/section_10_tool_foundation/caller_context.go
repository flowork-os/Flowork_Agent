// Package tools — caller_context.go
//
// Helper untuk pass caller_id (flowork-id://) lewat context.Context dari
// chat path → tool execution path. Di-pakai oleh tool yang per-user (mis.
// hak_warga.tool_propose, tools_per_id execution) untuk identify owner.
//
// Usage:
//
//	ctx = tools.WithCallerID(ctx, "flowork-id://superadmin:abc")
//	tools.Run(ctx, caps, "hak_warga.tool_propose", args)
//
//	// Di tool's Run():
//	caller := tools.CallerIDFromContext(ctx)
//	if caller != "" { /* per-user logic */ }

package tools

import "context"

type ctxKey string

const callerIDKey ctxKey = "flowork.caller_id"

// WithCallerID return new context dengan caller_id attached.
func WithCallerID(ctx context.Context, callerID string) context.Context {
	if callerID == "" {
		return ctx
	}
	return context.WithValue(ctx, callerIDKey, callerID)
}

// CallerIDFromContext extract caller_id. Empty kalau ngga di-set.
func CallerIDFromContext(ctx context.Context) string {
	v := ctx.Value(callerIDKey)
	if v == nil {
		return ""
	}
	id, _ := v.(string)
	return id
}
