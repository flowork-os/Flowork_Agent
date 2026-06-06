// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B2 mistakes recall. Verified: add 2x→hit_count, SearchMistakes
//   recall by context + hit_count order. Extend → file baru, JANGAN modify ini.
//
// mistakes_recall.go — Roadmap 2 Fase B2: tool mistake_recall.
//
// Sebelum ngerjain sesuatu yang beresiko keulang error, agent panggil ini buat
// cek "dulu gw pernah salah di konteks mirip ga?" → di-warn pakai remediation
// lampau. Plus tool mistake_log (existing) yang increment hit_count tiap error
// keulang. Pasangan log+recall = agent belajar dari kesalahan sendiri.
//
// On-demand (anti over-prompt) — bukan auto-inject.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

type mistakeRecallTool struct{}

func (mistakeRecallTool) Name() string       { return "mistake_recall" }
func (mistakeRecallTool) Capability() string { return "state:read" }
func (mistakeRecallTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Cek apakah lo PERNAH salah di konteks mirip (recall mistakes journal lo sendiri). Panggil SEBELUM ngerjain hal beresiko / yang pernah bermasalah, biar ga ngulang error yang sama. Balik daftar 'dulu lo salah X (Nx), solusinya Y' urut paling sering keulang.",
		Params: []tools.Param{
			{Name: "context", Type: tools.ParamString, Description: "deskripsi singkat situasi/tugas sekarang (kata kunci)", Required: true},
			{Name: "limit", Type: tools.ParamInt, Description: "max hasil (default 5, max 20)"},
		},
		Returns: "{count, warnings:[{title, remediation, hit_count, category}]}",
	}
}

func (mistakeRecallTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	q, _ := args["context"].(string)
	if strings.TrimSpace(q) == "" {
		return tools.Result{}, fmt.Errorf("context required")
	}
	limit := 0
	switch v := args["limit"].(type) {
	case float64:
		limit = int(v)
	case int:
		limit = v
	}
	hits, err := store.SearchMistakes(q, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("mistake_recall: %w", err)
	}
	warnings := make([]map[string]any, 0, len(hits))
	for _, m := range hits {
		warnings = append(warnings, map[string]any{
			"title":       m.Title,
			"remediation": m.Content,
			"hit_count":   m.HitCount,
			"category":    m.Category,
		})
	}
	note := ""
	if len(warnings) > 0 {
		note = "ada riwayat kesalahan mirip — baca remediation biar ga ngulang"
	}
	return tools.Result{Output: map[string]any{
		"count":    len(warnings),
		"warnings": warnings,
	}, Note: note}, nil
}
