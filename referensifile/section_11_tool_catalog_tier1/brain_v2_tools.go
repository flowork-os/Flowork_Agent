// Tools yang ngasih warga AI akses ke Brain v2 capability — abstraction layer
// di atas internal/brain/* sehingga warga ga perlu tau schema/SQL.
//
// Security: zero-trust per Ayah 2026-04-24 doktrin —
// "AI ngak boleh tahu sistem memori kita. Jika kelak ada AI jahat, dia
// bisa merusak. Jadi semua harus pake tools ini demi keamanan warga."
//
// Tools:
//   - brain_search        — hybrid FTS5+BM25 search drawer verbatim
//   - brain_recall        — L2 on-demand retrieval per topic (wing+room)


//   - brain_get_drawer    — ambil 1 drawer full content by ID
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	brainv2 "github.com/teetah2402/flowork/internal/brain"
	"github.com/teetah2402/flowork/internal/provider"
)

// ── BrainSearchTool ────────────────────────────────────────────────────────
// Hybrid search via FTS5 + BM25 ranking. Cari pengetahuan verbatim di drawers.

type BrainSearchTool struct{ workspace string }

type brainSearchArgs struct {
	Query string `json:"query" validate:"required"`
	Wing  string `json:"wing,omitempty"`
	Room  string `json:"room,omitempty"`
	N     int    `json:"n,omitempty"`
}

func NewBrainSearchTool(workspace string) *BrainSearchTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &BrainSearchTool{workspace: workspace}
}

func (t *BrainSearchTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "brain_search",
		Description: "Cari pengetahuan verbatim di Memory Palace via FTS5 hybrid search. " +
			"Pakai ini sebelum tanya ke LLM external — kalau jawaban udah ada di drawer, " +
			"ga perlu burning token. Return: list snippet dengan score relevansi 0-1.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Query search dalam bahasa natural. Min 2 char.",
				},
				"wing": map[string]any{
					"type":        "string",
					"description": "Optional filter: wing (proyek/agent name, e.g. 'merpati', 'general').",
				},
				"room": map[string]any{
					"type":        "string",
					"description": "Optional filter: room (topic spesifik, e.g. 'telegram-acl').",
				},
				"n": map[string]any{
					"type":        "integer",
					"description": "Max results (default 10).",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *BrainSearchTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args brainSearchArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("validation: %w", err)
	}
	if args.N == 0 {
		args.N = 10
	}
	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}
	results, err := brainv2.HybridSearch(db, args.Query, args.Wing, args.Room, args.N)
	if err != nil {
		return Result{}, fmt.Errorf("search: %w", err)
	}
	if len(results) == 0 {
		return Result{
			ToolName: "brain_search",
			OK:       true,
			Output:   fmt.Sprintf("Tidak ada drawer match untuk query %q. Coba kata kunci lain, atau jalankan brain_search dulu untuk topik luas — kalau memang belum di-mine, jawab dari LLM normal.", args.Query),
		}, nil
	}
	out, _ := json.MarshalIndent(map[string]any{
		"query":   args.Query,
		"count":   len(results),
		"results": results,
	}, "", "  ")
	return Result{ToolName: "brain_search", OK: true, Output: string(out)}, nil
}

// ── BrainRecallTool ────────────────────────────────────────────────────────
// L2 on-demand retrieval — pre-load konteks untuk topic tertentu.

type BrainRecallTool struct{ workspace string }

type brainRecallArgs struct {
	Wing string `json:"wing" validate:"required"`
	Room string `json:"room,omitempty"`
}

func NewBrainRecallTool(workspace string) *BrainRecallTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &BrainRecallTool{workspace: workspace}
}

func (t *BrainRecallTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "brain_recall",
		Description: "Recall semua drawer terkait wing+room (Layer 2 memory). " +
			"Pakai saat lo udah tau topic spesifik dan butuh konteks lengkap, " +
			"bukan keyword search. Return: text gabungan drawer top-10 by importance.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"wing": map[string]any{
					"type":        "string",
					"description": "Wing/proyek (mandatory, e.g. 'merpati', 'general', 'flowork').",
				},
				"room": map[string]any{
					"type":        "string",
					"description": "Optional room/topic narrower scope.",
				},
			},
			"required": []string{"wing"},
		},
	}
}

func (t *BrainRecallTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args brainRecallArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("validation: %w", err)
	}
	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}
	stack := brainv2.NewMemoryStack(db, args.Wing, "")
	content := stack.Recall(args.Room)
	if content == "" {
		return Result{
			ToolName: "brain_recall",
			OK:       true,
			Output:   fmt.Sprintf("Belum ada drawer untuk wing=%q room=%q. Coba mining recordings dulu (POST /api/brain/v2/mine) atau cek tab Memory Palace.", args.Wing, args.Room),
		}, nil
	}
	return Result{ToolName: "brain_recall", OK: true, Output: content}, nil
}

// ── BrainGetDrawerTool ─────────────────────────────────────────────────────
// Ambil 1 drawer full content (kalau brain_search nemu hasil partial dan butuh detail lengkap).

type BrainGetDrawerTool struct{ workspace string }

type drawerArgs struct {
	ID string `json:"id" validate:"required"`
}

func NewBrainGetDrawerTool(workspace string) *BrainGetDrawerTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &BrainGetDrawerTool{workspace: workspace}
}

func (t *BrainGetDrawerTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "brain_get_drawer",
		Description: "Ambil 1 drawer full content by ID. " +
			"Pakai setelah brain_search return snippet — kalau snippet menjanjikan tapi terpotong, " +
			"call ini dengan drawer_id buat content lengkap.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Drawer ID (16-char hex prefix dari content_hash).",
				},
			},
			"required": []string{"id"},
		},
	}
}

func (t *BrainGetDrawerTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args drawerArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("validation: %w", err)
	}
	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}
	d, err := brainv2.GetDrawer(db, strings.TrimSpace(args.ID))
	if err != nil {
		return Result{
			ToolName: "brain_get_drawer",
			OK:       false,
			Output:   fmt.Sprintf("Drawer %q tidak ditemukan: %v", args.ID, err),
		}, nil
	}
	out, _ := json.MarshalIndent(d, "", "  ")
	return Result{ToolName: "brain_get_drawer", OK: true, Output: string(out)}, nil
}
