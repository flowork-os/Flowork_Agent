// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B6 federation. Verified: promote local->router shared (findable
//   by others), quality-gate (quarantine excluded), anti double-promote, resilient
//   (router down graceful). Extend -> file baru, JANGAN modify ini.
//
// brain_federation.go — Roadmap 2 Fase B6: tool brain_promote_shared.
//
// Orchestrasi federation: pilih drawer lokal yang LAYAK (quality-gate di agentdb)
// → push ke router shared brain (routerclient) → catat sync log. OPSIONAL +
// resilient: router mati → tandai 'error', SKIP, agent tetep jalan. Memanggil
// tool ini = bentuk owner/agent-approve buat share knowledge.

package builtins

import (
	"context"
	"fmt"

	"flowork-gui/internal/routerclient"
	"flowork-gui/internal/tools"
)

type brainPromoteSharedTool struct{}

func (brainPromoteSharedTool) Name() string       { return "brain_promote_shared" }
func (brainPromoteSharedTool) Capability() string { return "rpc:router:brain" }
func (brainPromoteSharedTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Share knowledge brain LOKAL lo yang berharga ke korpus SHARED di router (biar warga lain bisa belajar). Quality-gate: cuma drawer non-karantina, confidence tinggi, tipe aman (experience/eureka/fact) — constitution/secret GA di-share. Resilient: kalau router mati, di-skip. Balik jumlah yang ke-share.",
		Params: []tools.Param{
			{Name: "limit", Type: tools.ParamInt, Description: "max drawer di-share sekali jalan (default 20, max 100)"},
		},
		Returns: "{eligible, promoted, skipped, router_ok}",
	}
}

func (brainPromoteSharedTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	limit := 0
	switch v := args["limit"].(type) {
	case float64:
		limit = int(v)
	case int:
		limit = v
	}

	cands, err := store.SelectPromotable(limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("brain_promote_shared: %w", err)
	}
	if len(cands) == 0 {
		return tools.Result{Output: map[string]any{
			"eligible": 0, "promoted": 0, "skipped": 0, "router_ok": true,
		}, Note: "ga ada drawer baru yang layak di-share"}, nil
	}

	// Resolve router URL dari config agent (mirror brain.go).
	routerURL := routerclient.DefaultRouterURL
	if cfg, lerr := store.Load(); lerr == nil {
		if u, ok := cfg["router_url"].(string); ok && u != "" {
			routerURL = u
		}
	}
	client := routerclient.New(routerURL)

	promoted, skipped := 0, 0
	routerOK := true
	for _, d := range cands {
		resp, perr := client.PromoteDrawer(ctx, routerclient.PromoteDrawerReq{
			Content: d.Content, Wing: d.Wing, Room: d.Room, MemType: d.MemType,
		})
		if perr != nil {
			// Router mati / error → tandai error, SKIP. Agent jalan terus.
			_ = store.MarkPromoted(d.ID, "", "error")
			skipped++
			routerOK = false
			continue
		}
		_ = store.MarkPromoted(d.ID, resp.ID, "ok")
		promoted++
	}
	note := ""
	if !routerOK {
		note = "sebagian/semua gagal — router mungkin mati. Aman, agent tetep jalan (brain lokal)."
	}
	return tools.Result{Output: map[string]any{
		"eligible":  len(cands),
		"promoted":  promoted,
		"skipped":   skipped,
		"router_ok": routerOK,
	}, Note: note}, nil
}
