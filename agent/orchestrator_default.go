// orchestrator_default.go — GROWTH-POINT (NON-frozen). SATU sumber kebenaran buat
// "agent default yang di-ajak ngobrol semua channel" (HTTP /api/chat, CLI, MCP,
// connector). Dulu default ini ke-hardcode TERSEBAR + sebagian nunjuk "mr-flow-next"
// yang BELUM ke-deploy → /api/chat default mati ("agent mr-flow-next not loaded").
//
// AKAR yang dicabut (owner 2026-06-25, "selesaikan dari akar"): default balik ke
// mr-flow (orchestrator LIVE), TAPI lewat SWITCH biar evolusi ga ke-batasin —
// pas mr-flow-next (atau orchestrator baru) udah beneran ke-deploy, cukup set ENV,
// NOL ubah/buka kode lagi (Rule 7). Detail: lock/mrflow.md §6b.
//
//	SWITCH: ENV FLOWORK_ORCHESTRATOR=<agent-id>  (kosong = "mr-flow")
package main

import (
	"os"
	"strings"
)

// defaultOrchestratorID — id agent default yang di-ajak ngobrol channel. ENV-overridable.
func defaultOrchestratorID() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ORCHESTRATOR")); v != "" {
		return v
	}
	return "mr-flow"
}
