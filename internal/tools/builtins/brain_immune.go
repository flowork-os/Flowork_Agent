// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B5 immune. Verified: antibody match -> quarantine -> excluded
//   from brain_search, normal kept, verify releases. Extend -> file baru.
//
// brain_immune.go — Roadmap 2 Fase B5: tools immune system brain.
//
// brain_immune_scan : sapu brain lokal → quarantine drawer injection/halu/low-conf.
// brain_verify       : rilis drawer dari quarantine setelah di-verify aman.
// Quarantined drawer ga muncul di brain_search (SearchLocalBrain filter).

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

type brainImmuneScanTool struct{}

func (brainImmuneScanTool) Name() string       { return "brain_immune_scan" }
func (brainImmuneScanTool) Capability() string { return "state:write" }
func (brainImmuneScanTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Sapu brain LOKAL lo → karantina drawer yang kena pola injection/jailbreak (antibody) atau confidence rendah, biar brain ga keracunan halu. Balik jumlah yang dikarantina + daftar yang lagi dikarantina.",
		Params:      nil,
		Returns:     "{quarantined_now, quarantined_total:[{id, reason_quarantine, content}]}",
	}
}

func (brainImmuneScanTool) Run(ctx context.Context, _ map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	n, err := store.ScanAndQuarantine()
	if err != nil {
		return tools.Result{}, fmt.Errorf("brain_immune_scan: %w", err)
	}
	list, _ := store.ListQuarantined(20)
	return tools.Result{Output: map[string]any{
		"quarantined_now":   n,
		"quarantined_total": list,
	}}, nil
}

type brainVerifyTool struct{}

func (brainVerifyTool) Name() string       { return "brain_verify" }
func (brainVerifyTool) Capability() string { return "state:write" }
func (brainVerifyTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Rilis 1 drawer brain LOKAL dari karantina setelah lo yakin isinya aman (bukan injection/halu). Set confidence baru (default 1.0). Habis ini drawer-nya muncul lagi di brain_search.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamString, Description: "drawer id (dari brain_immune_scan)", Required: true},
			{Name: "confidence", Type: tools.ParamFloat, Description: "tier-confidence 0..1 (default 1.0)"},
		},
		Returns: "{id, verified: true}",
	}
}

func (brainVerifyTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	id, _ := args["id"].(string)
	if strings.TrimSpace(id) == "" {
		return tools.Result{}, fmt.Errorf("id required")
	}
	conf := 0.0
	switch v := args["confidence"].(type) {
	case float64:
		conf = v
	case int:
		conf = float64(v)
	}
	if err := store.VerifyDrawer(id, conf); err != nil {
		return tools.Result{}, fmt.Errorf("brain_verify: %w", err)
	}
	return tools.Result{Output: map[string]any{"id": id, "verified": true}}, nil
}
