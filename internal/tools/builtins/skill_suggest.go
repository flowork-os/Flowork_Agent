// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B4 skill grow-from-patterns. Verified: successful tool freq
//   -> candidates (failed/rare excluded). Pair w/ Fase 8 curator. Extend -> file baru.
//
// skill_suggest.go — Roadmap 2 Fase B4: tool skill_suggest.
//
// Lihat pola tool sukses berulang lo → usulan bikin skill. On-demand. Pasangan
// sama curator (Fase 8) yang grade/consolidate/archive skill existing. Bareng:
// skill TUMBUH dari pola sukses + ke-CURATE biar ga basi.

package builtins

import (
	"context"
	"fmt"

	"flowork-gui/internal/tools"
)

type skillSuggestTool struct{}

func (skillSuggestTool) Name() string       { return "skill_suggest" }
func (skillSuggestTool) Capability() string { return "state:read" }
func (skillSuggestTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Lihat usulan SKILL dari pola tool yang sering lo pakai SUKSES. Pakai buat refleksi: tool apa yang udah jadi kebiasaan sukses → bisa diformalin jadi skill/alur. Balik kandidat urut paling sering.",
		Params: []tools.Param{
			{Name: "min_count", Type: tools.ParamInt, Description: "minimal jumlah sukses biar jadi kandidat (default 2)"},
			{Name: "limit", Type: tools.ParamInt, Description: "max kandidat (default 10, max 50)"},
		},
		Returns: "{count, candidates:[{tool_name, success_count, last_used, suggestion}]}",
	}
}

func (skillSuggestTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	minCount := 0
	limit := 0
	switch v := args["min_count"].(type) {
	case float64:
		minCount = int(v)
	case int:
		minCount = v
	}
	switch v := args["limit"].(type) {
	case float64:
		limit = int(v)
	case int:
		limit = v
	}
	cands, err := store.SuggestSkillCandidates(minCount, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("skill_suggest: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"count":      len(cands),
		"candidates": cands,
	}}, nil
}
