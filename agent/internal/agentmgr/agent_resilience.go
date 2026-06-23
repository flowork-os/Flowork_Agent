// 🔐 LOCKED (stabil 2026-06-23, BUKAN freeze) — edit dgn izin owner. Tuning cepat tanpa edit: env
// FLOWORK_RESILIENCE_OFF=1. Roadmap: docs/ROADMAP_AGENT_RESILIENCE.md ITEM 1.
//
// agent_resilience.go — DOKTRIN RECOVERY buat SEMUA agent (ITEM 1 roadmap agent-resilience, 2026-06-23).
//
// MASALAH: agent mati + muter pas error transient (fbspecial: "router error" → auto-continue #8).
// Prinsip ditiru dari Claude Code (prompts.ts:233/240): diagnosa-sebelum-ganti-taktik, jangan-ulang-buta,
// jangan-nyerah-1×, lapor-jujur, jangan-polling. Ini lapis PROMPT (kesadaran); lapis CONTROL-FLOW (retry-
// backoff) = ITEM 2. Di-inject ke system-prompt SEMUA agent tiap turn lewat SelfPromptRenderHandler
// (cabang non-frozen, pola sama WIBNowHeader). Sengaja RINGKAS (~110 token) — patuh hemat-token owner.
package agentmgr

import (
	"os"
	"strings"
)

// RecoveryDoctrine — blok ringkas "cara hadapi error/macet" buat tiap agent. Bikin agent tahan-banting:
// recover dari kegagalan transient, ga muter, ga nyerah dini, jujur. SWITCH: FLOWORK_RESILIENCE_OFF=1 → "".
func RecoveryDoctrine() string {
	if strings.TrimSpace(os.Getenv("FLOWORK_RESILIENCE_OFF")) == "1" {
		return ""
	}
	return "# CARA HADAPI ERROR / MACET\n" +
		"- Error → DIAGNOSA dulu (baca pesannya), jangan ulang buta / nyerah 1× gagal.\n" +
		"- Transient (router/timeout/server sibuk) → tunggu, COBA LAGI; jangan ngaku kelar gara2 blip.\n" +
		"- Macet beneran → jujur + tawarin opsi, JANGAN muter ngulang langkah sama.\n" +
		"- Belum kelar → LAPOR JUJUR, jangan ngaku selesai. Nunggu background → bakal dikabarin, jangan polling.\n\n"
}
