// 🔒 FROZEN tools-core — Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// JANGAN edit tanpa unfreeze SADAR owner (di-hash KERNEL_FREEZE.md + chattr +i). Baca lock/tools.md DULU.
// Team-review = Dewan self-evolution (CONFIGURABLE di GUI, bukan di sini). Perluasan = file feature baru.
//
// feature_tools_promote.go — FASE 2 SELF-EVOLVING: tool privat → Dewan review → SHARED.
//
// Owner 2026-06-23: tool buatan-agent lahir PRIVAT, lalu di-review TEAM (Dewan self-evolution yg udah
// ada — configurable, no-owner-acc), lolos → jadi shared semua agent. REUSE Dewan (ga bikin team baru):
//   - autoProposePrivateTools → bikin EvolveProposal kind "promote-tool" di store mr-flow (antrian Dewan).
//   - Dewan (cron drain + council adversarial) review → approve/reject.
//   - approve → evolveApplier case "promote-tool" → promoteToolApply → toolsidecar.Promote (pindah+shared).
//
// File NON-frozen (cabang) — selfevolve_apply.go cuma delegasi 1 baris ke sini. Blueprint: roadmap §15.
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/toolsidecar"
)

func init() {
	RegisterFeature(Feature{Name: "tools-promote", Phase: PhaseSeed, Apply: func(d *Deps) {
		if n, err := autoProposePrivateTools(d.Host); err == nil && n > 0 {
			log.Printf("tools-promote: %d tool privat di-propose ke Dewan (kind promote-tool)", n)
		}
		// Trigger scan manual (auto-propose tool privat baru ke antrian Dewan).
		d.Mux.HandleFunc("/api/tools/promote-scan", func(w http.ResponseWriter, r *http.Request) {
			n, err := autoProposePrivateTools(d.Host)
			if err != nil {
				httpx.WriteJSON(w, map[string]any{"error": err.Error()})
				return
			}
			httpx.WriteJSON(w, map[string]any{
				"proposed": n,
				"note":     "tool privat masuk antrian Dewan self-evolution (kind promote-tool). Lolos review → auto-promote shared (no-owner-acc).",
			})
		})
	}})
}

// promoteToolApply — dipanggil evolveApplier (case promote-tool) setelah Dewan APPROVE. Pindah tool
// privat → shared + re-register. p.TargetFile = path folder tool privat.
func promoteToolApply(p agentdb.EvolveProposal) (map[string]any, error) {
	name := strings.TrimSpace(filepath.Base(strings.TrimSpace(p.TargetFile)))
	if name == "" {
		return nil, fmt.Errorf("promote-tool: nama tool ga kebaca dari target_file %q", p.TargetFile)
	}
	res, err := toolsidecar.Promote(toolsidecar.ToolsDir(), name)
	if err != nil {
		return nil, fmt.Errorf("promote tool %q: %w", name, err)
	}
	res["note"] = "Tool '" + name + "' LOLOS Dewan → sekarang SHARED (semua agent bisa pake)."
	return res, nil
}

// autoProposePrivateTools — tiap tool privat yg belum di-antri → bikin EvolveProposal "promote-tool"
// di store mr-flow (sumber Dewan). Idempoten (ID = promote-tool:<name>, ON CONFLICT ga reset status).
func autoProposePrivateTools(host *kernelhost.Host) (int, error) {
	if host == nil {
		return 0, fmt.Errorf("nil host")
	}
	store, err := host.OpenAgentStore("mr-flow") // store sumber Dewan (selfevolve openAgentStore defaultAgentID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range toolsidecar.PrivateList() {
		_, dir, ok := toolsidecar.PrivateInfo(s.Name)
		if !ok {
			continue
		}
		p := agentdb.EvolveProposal{
			ID:         "promote-tool:" + s.Name,
			Kind:       "promote-tool",
			Goal:       "Promote tool '" + s.Name + "' jadi shared",
			TargetFile: dir,
			Rationale: "Tool sidecar privat '" + s.Name + "' (buatan " + s.AgentID + "): " + s.Description +
				". Usul jadi SHARED biar semua agent bisa pake — hemat (ga bikin ulang), dorong otonomi + ekonomi. " +
				"Aman: lolos build-verify + anti-eskalasi, jalan sbg proses sidecar terpisah.",
			Risk:   "low",
			Pillar: "ekonomi",
			Status: "proposed",
		}
		if store.AddEvolveProposal(p) == nil {
			n++
		}
	}
	return n, nil
}
