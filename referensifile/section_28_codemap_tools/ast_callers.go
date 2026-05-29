// Package tools — `find_callers` tool untuk reverse lookup call-sites
// (L-02 per hasil_audit_antygravity_opus_keren.md).
//
// Complement `search_semantic_function` (forward: name → declaration).
// Reverse: name → call-sites yang invoke itu. Backend: factmemory.CallGraph.
//
// Index dibangun on-demand kalau state/factmemory/ast_calls.json belum ada
// — konsisten dengan pattern AstSearchTool (lazy BuildIndex).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/factmemory"
	"github.com/teetah2402/flowork/internal/provider"
)

// FindCallersTool menjawab "dimana fungsi X dipanggil?".
type FindCallersTool struct {
	workspace string
}

type findCallersArgs struct {
	Query string `json:"query" validate:"required"`
	Limit int    `json:"limit,omitempty"`
}

// NewFindCallersTool builds a new tool instance bound to workspace.
func NewFindCallersTool(workspace string) *FindCallersTool {
	return &FindCallersTool{workspace: workspace}
}

// Definition exposes the tool metadata to the LLM provider.
func (t *FindCallersTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "find_callers",
		Description: `Reverse semantic lookup: cari semua call-sites yang memanggil fungsi/method bernama X di seluruh repositori Go.

Komplemen untuk 'search_semantic_function' (yang cari deklarasi). Tool ini jawab pertanyaan: "kalau gw rename/refactor fungsi ini, siapa yang terpengaruh?"

Query bisa berupa:
  - Nama plain: "Open" (match "Open", "pkg.Open", "Type.Open")
  - Qualified: "mempool.Stage" (match exact)

Returns: daftar caller fungsi + filepath:line per call-site.

First-run kalau index belum ada, akan build ulang call-graph (~1-3 detik untuk repo ~200 file).`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Nama fungsi/method yang mau dicari call-site-nya (e.g. 'Stage', 'runHealer', 'aiforge.Router')",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (default 100). Ideal untuk narrow down kalau query terlalu umum.",
				},
			},
			"required": []string{"query"},
		},
	}
}

// Execute runs the lookup. Rebuilds call graph on-demand kalau missing.
func (t *FindCallersTool) Execute(_ context.Context, invocation Invocation) (Result, error) {
	var args findCallersArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return Result{}, fmt.Errorf("query required")
	}
	if args.Limit <= 0 {
		args.Limit = 100
	}

	graph, err := factmemory.LoadCallGraph(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("load call graph: %w", err)
	}
	// Lazy build: kalau graph empty, (re)build sekarang.
	if graph == nil || len(graph.Calls) == 0 {
		graph, err = factmemory.BuildCallGraph(t.workspace)
		if err != nil {
			return Result{}, fmt.Errorf("build call graph: %w", err)
		}
	}

	callers := graph.FindCallers(args.Query)
	if len(callers) == 0 {
		return Result{
			Output: fmt.Sprintf("Tidak ada call-site untuk %q. Kemungkinan: dead code, fungsi baru belum dipakai, atau nama berbeda (coba search_semantic_function dulu untuk cek deklarasi).", args.Query),
		}, nil
	}

	if len(callers) > args.Limit {
		callers = callers[:args.Limit]
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%d call-site(s) found for %q:", len(callers), args.Query))
	for _, c := range callers {
		lines = append(lines, fmt.Sprintf("  %s:%d  %s → %s", c.Filepath, c.Line, c.Caller, c.Callee))
	}
	return Result{
		Output: strings.Join(lines, "\n"),
		Metadata: map[string]any{
			"count":   len(callers),
			"callers": callers,
		},
	}, nil
}
