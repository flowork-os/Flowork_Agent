// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B0 tools brain lokal. E2E verified (mr-flow brain_add →
//   brain_search recall via pipeline). Extend (brain_forget/brain_promote) →
//   tambah file baru, JANGAN modify ini.
//
// brain_local.go — Roadmap 2 Fase B0: tools brain LOKAL per-agent.
//
// 3 tool yang nyentuh brain lokal di state.db (agentdb brain_drawers.go):
//   - brain_add    : simpen knowledge/experience ke brain SENDIRI (state:write)
//   - brain_search : cari di brain SENDIRI pakai FTS5 lokal (state:read) — MURAH,
//                    no router, no embedding. Ini default "inget pengalaman gw".
//   - brain_get    : ambil 1 drawer full by id (state:read)
//
// Layered (roadmap 1.4/1.5): brain_search = LOKAL (pengalaman sendiri).
// brain_search_shared (brain.go) = router 5jt (korpus luas, on-demand).
// Local-first; shared optional. Router mati → brain_search lokal tetep jalan.
//
// Pola: tools.FromStore(ctx) + store.AddBrainDrawer/SearchLocalBrain/GetBrainDrawer.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

// ── brain_add ───────────────────────────────────────────────────────────────

type brainAddTool struct{}

func (brainAddTool) Name() string       { return "brain_add" }
func (brainAddTool) Capability() string { return "state:write" }
func (brainAddTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Simpan knowledge/pengalaman ke brain LOKAL lo sendiri (state.db, FTS5). Pakai buat inget hal penting hasil sendiri: pola sukses, fakta, kesimpulan, eureka. Dedup otomatis (content sama ga dobel). Ini brain PRIBADI lo — beda dari brain_search_shared (korpus router). Recall-nya pakai brain_search.",
		Params: []tools.Param{
			{Name: "content", Type: tools.ParamString, Description: "isi knowledge/pengalaman (ringkas, faktual)", Required: true},
			{Name: "wing", Type: tools.ParamString, Description: "kategori besar (default 'general', mis. experience/fact/eureka)"},
			{Name: "room", Type: tools.ParamString, Description: "sub-kategori opsional"},
			{Name: "mem_type", Type: tools.ParamString, Description: "tipe memori (default 'experience')"},
		},
		Returns: "{id, added, deduped}",
	}
}

func (brainAddTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return tools.Result{}, fmt.Errorf("content required")
	}
	wing, _ := args["wing"].(string)
	room, _ := args["room"].(string)
	memType, _ := args["mem_type"].(string)

	id, added, err := store.AddBrainDrawer(content, wing, room, memType, "agent")
	if err != nil {
		return tools.Result{}, fmt.Errorf("brain_add: %w", err)
	}
	note := ""
	if !added {
		note = "drawer sudah ada (dedup by content) — ga di-insert ulang"
	}
	return tools.Result{Output: map[string]any{
		"id":      id,
		"added":   added,
		"deduped": !added,
	}, Note: note}, nil
}

// ── brain_search (LOKAL) ──────────────────────────────────────────────────────

type brainSearchLocalTool struct{}

func (brainSearchLocalTool) Name() string       { return "brain_search" }
func (brainSearchLocalTool) Capability() string { return "state:read" }
func (brainSearchLocalTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Cari di brain LOKAL lo sendiri (pengalaman/knowledge yang LO simpan via brain_add) pakai FTS5 BM25. Murah, cepat, no router. Pakai ini DULU buat 'inget pengalaman/knowledge gw soal X'. Kalau butuh korpus pengetahuan luas (security/training/dll), pakai brain_search_shared.",
		Params: []tools.Param{
			{Name: "query", Type: tools.ParamString, Description: "kata kunci / pertanyaan", Required: true},
			{Name: "k", Type: tools.ParamInt, Description: "max hasil (default 5, max 10)"},
		},
		Returns: "{query, hits:[{drawer_id, wing, room, mem_type, content, score}], count}",
	}
}

func (brainSearchLocalTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return tools.Result{}, fmt.Errorf("query required")
	}
	k := 0
	switch v := args["k"].(type) {
	case float64:
		k = int(v)
	case int:
		k = v
	}
	hits, err := store.SearchLocalBrain(query, k)
	if err != nil {
		return tools.Result{}, fmt.Errorf("brain_search: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"query": query,
		"hits":  hits,
		"count": len(hits),
	}}, nil
}

// ── brain_get ─────────────────────────────────────────────────────────────────

type brainGetTool struct{}

func (brainGetTool) Name() string       { return "brain_get" }
func (brainGetTool) Capability() string { return "state:read" }
func (brainGetTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Ambil 1 drawer full dari brain LOKAL by id (id dari hasil brain_search).",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamString, Description: "drawer id", Required: true},
		},
		Returns: "{found, drawer}",
	}
}

func (brainGetTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	id, _ := args["id"].(string)
	if strings.TrimSpace(id) == "" {
		return tools.Result{}, fmt.Errorf("id required")
	}
	d, found, err := store.GetBrainDrawer(id)
	if err != nil {
		return tools.Result{}, fmt.Errorf("brain_get: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"found":  found,
		"drawer": d,
	}}, nil
}
