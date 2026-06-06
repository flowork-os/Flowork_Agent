// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 10 (Tool foundation interface) phase 1 DONE. API
//   stable: Tool interface (Name/Schema/Capability/Run), Schema struct,
//   Param/ParamType enum, Result struct, MarshalArgs/MarshalResult
//   helpers. Phase 2 enhancements (deadline ctx, mid-run hooks, stream
//   output) → tambah optional interface di file lain, JANGAN modify ini.
//
// Package tools — Section 10 phase 1: tool system foundation.
//
// PURPOSE:
//   Skeleton untuk tool dispatch system. Phase 1 = interface + registry +
//   invocation log. Tools beneran (file/shell/web/etc) di-implement Section
//   11 Tier 1.
//
// DESIGN:
//   - Setiap tool implement `Tool` interface (Name, Schema, Run).
//   - Plug-and-play: register di `init()` pakai tools.Register(t).
//   - Caller dispatch via tools.Lookup(name).Run(ctx, args).
//   - Permission gate (broker cap check) di-wire phase 2.
//
// Source: Flowork_Agent/roadmap.md Section 10 phase 1.

package tools

import (
	"context"
	"encoding/json"
)

// AlgoVersion — tool system schema version.
const AlgoVersion = "v1"

// ParamType — primitive type buat schema arg.
type ParamType string

const (
	ParamString ParamType = "string"
	ParamInt    ParamType = "int"
	ParamFloat  ParamType = "float"
	ParamBool   ParamType = "bool"
	ParamObject ParamType = "object" // arbitrary JSON
	ParamArray  ParamType = "array"
)

// Param — single tool arg spec.
type Param struct {
	Name        string    `json:"name"`
	Type        ParamType `json:"type"`
	Description string    `json:"description"`
	Required    bool      `json:"required"`
	Default     any       `json:"default,omitempty"`
}

// Schema — tool input/output spec. Anti over-prompt: keep summary minimal,
// caller pull detail Schema on-demand via list endpoint.
type Schema struct {
	Description string  `json:"description"`
	Params      []Param `json:"params"`
	Returns     string  `json:"returns,omitempty"` // human description of return shape
}

// Result — tool execution outcome. Output sebagai JSON-serializable any.
type Result struct {
	Output any    `json:"output"`
	Note   string `json:"note,omitempty"` // optional human note (warning, fallback used, etc)
}

// Tool — interface yang setiap tool implementation harus penuhi.
type Tool interface {
	// Name — unique identifier, format `verb_noun` (mis. `read_file`, `bash_run`).
	Name() string
	// Schema — declare input params + output description.
	Schema() Schema
	// Capability — capability string required (mis. `fs:read`, `exec:shell`,
	// `net:fetch:https://*`). Broker cek warga punya cap ini sebelum dispatch.
	// Phase 1: no enforcement yet (defer phase 2).
	Capability() string
	// Run — execute. args di-marshal sesuai Schema. Caller ctx untuk cancel.
	Run(ctx context.Context, args map[string]any) (Result, error)
}

// MarshalArgs — helper marshal map ke JSON string (for logging).
func MarshalArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// MarshalResult — helper marshal result ke JSON string (for logging).
func MarshalResult(r Result) string {
	b, err := json.Marshal(r)
	if err != nil {
		return "{}"
	}
	return string(b)
}
