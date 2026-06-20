// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-20 (bridge ide owner→evolusi).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner).
package builtins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

// evolve_propose_tool.go — owner 2026-06-20: "mr-flow kasih evolve_proposals; gw aktif diskusi
// sama mr-flow, dia salurkan IDE GW ke team evolute biar di-review". Tool ini bikin mr-flow bisa
// nyetor ide owner langsung jadi proposal evolusi (status=proposed) → masuk pipeline normal:
// DEWAN review → core-apply (loop coder↔reviewer) → STAGE. Ditandai [IDE OWNER] biar prioritas +
// ke-skip auto-reject classifier (classifier cuma jalan di jalur proposer/reflect, BUKAN di sini).

func init() { tools.Register(&evolveProposeTool{}) }

type evolveProposeTool struct{}

func (evolveProposeTool) Name() string       { return "evolve_propose" }
func (evolveProposeTool) Capability() string { return "state:write" }
func (evolveProposeTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Salurkan IDE OWNER jadi proposal evolusi (masuk backlog → Dewan review → core-apply). Pakai pas owner kasih ide perbaikan/fitur lewat chat & minta diteruskan ke team evolusi. Ditandai [IDE OWNER] (prioritas, ga kena auto-reject classifier).",
		Params: []tools.Param{
			{Name: "target_file", Type: tools.ParamString, Description: "file yang disentuh, relatif repo. File BARU pakai prefix 'NEW:' (mis. NEW:internal/agentdb/foo.go)", Required: true},
			{Name: "rationale", Type: tools.ParamString, Description: "kenapa ide ini penting (ringkas, 1-2 kalimat)", Required: true},
			{Name: "kind", Type: tools.ParamString, Description: "jenis: add-agent|add-skill|add-app (behavior) atau fix|refactor|doc|test (core). Default refactor"},
			{Name: "goal", Type: tools.ParamString, Description: "konteks/tujuan ide (optional)"},
			{Name: "risk", Type: tools.ParamString, Description: "low|medium|high (default medium)"},
		},
		Returns: "{ok, id, status, pillar}",
	}
}

func (evolveProposeTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	target := strings.TrimSpace(asString(args["target_file"]))
	rationale := strings.TrimSpace(asString(args["rationale"]))
	if target == "" || rationale == "" {
		return tools.Result{}, fmt.Errorf("target_file & rationale wajib")
	}
	kind := strings.TrimSpace(asString(args["kind"]))
	if kind == "" {
		kind = "refactor"
	}
	risk := strings.ToLower(strings.TrimSpace(asString(args["risk"])))
	if risk != "low" && risk != "high" {
		risk = "medium"
	}
	goal := strings.TrimSpace(asString(args["goal"]))
	if goal == "" {
		goal = "ide owner via mr-flow"
	}

	var b [8]byte
	_, _ = rand.Read(b[:])
	id := "ev_owner_" + hex.EncodeToString(b[:])
	// Pilar otomatis biar ke-tag (bukan "ngelantur") — ide owner tetep relevan ke 5 pilar.
	pillars := agentdb.ClassifyPillars(goal + " " + kind + " " + rationale + " " + target)
	p := agentdb.EvolveProposal{
		ID: id, Goal: "[IDE OWNER] " + goal, TargetFile: target, Kind: kind,
		Rationale: "[IDE OWNER — prioritas, sudah di-vouch owner] " + rationale,
		Risk:      risk, Status: "proposed", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Pillar: strings.Join(pillars, ","),
	}
	if err := store.AddEvolveProposal(p); err != nil {
		return tools.Result{}, fmt.Errorf("simpan proposal: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"ok": true, "id": id, "status": "proposed", "pillar": p.Pillar,
		"note": "Ide owner masuk backlog evolusi → bakal di-review Dewan + (kalau approved) di-code+review team. Cek tab Evolution.",
	}}, nil
}
