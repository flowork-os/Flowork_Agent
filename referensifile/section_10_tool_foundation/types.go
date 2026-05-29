// Package tools menyediakan tool registry terpadu, interceptor, dan implementasi built-in tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/teetah2402/flowork/internal/provider"
)

// State mendeskripsikan context state saat satu tool dijalankan.
type State struct {
	WorkingDir string
	SessionID  string
	// AgentID — caller warga identity di-set dispatcher (HTTP header
	// X-Flowork-Agent atau body field). Propagated ke Invocation.AgentID
	// supaya interceptor (karma check) pakai per-request identity, bukan
	// env var single-process. Per-request context fix 2026-05-11.
	AgentID string
}

// Invocation merepresentasikan satu tool call request yang sudah diparse.
type Invocation struct {
	ToolName   string
	CallID     string
	WorkingDir string
	SessionID  string
	Arguments  json.RawMessage
	ParsedArgs map[string]any

	// AgentID — caller warga identity. Diisi dispatcher dari HTTP header
	// `X-Flowork-Agent` atau request body field `agent`/`caller_id`. Empty
	// = fallback ke env var FLOWORK_AGENT_NAME atau 'default' (legacy single-process behavior).
	// Per-request context fix 2026-05-11 (Ayah QC found karma bug pakai 'default').
	AgentID string
}

// Result merepresentasikan hasil terpadu setelah eksekusi tool selesai.
type Result struct {
	ToolName string         `json:"tool_name"`
	OK       bool           `json:"ok"`
	Output   string         `json:"output" validate:"required"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Render merender tool result menjadi teks JSON yang cocok dikirim kembali ke model.
func (r Result) Render() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"output":"%s"}`, err.Error())
	}
	return string(data)
}

// Tool mendefinisikan interface terpadu yang harus diimplementasikan built-in tool.
type Tool interface {
	// Definition mengembalikan deskripsi tool yang diexpose ke model.
	Definition() provider.ToolDefinition
	// Execute menjalankan satu pemanggilan tool.
	Execute(ctx context.Context, invocation Invocation) (Result, error)
}

// Interceptor mendefinisikan extension point untuk mencegat sebelum dan sesudah eksekusi tool.
type Interceptor interface {
	// Before dipanggil sebelum eksekusi tool, berguna untuk validasi, audit, atau blokir pemanggilan.
	Before(ctx context.Context, invocation *Invocation) error
	// After dipanggil setelah eksekusi tool, berguna untuk mencatat hasil atau melengkapi side effect.
	After(ctx context.Context, invocation Invocation, result *Result, err error)
}
