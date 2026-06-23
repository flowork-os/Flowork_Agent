// cognitive_ext.go — CABANG (extension point) NON-FROZEN buat Cognitive Graph (CGM).
//
// ⚖️ ATURAN ABADI (owner Mr.Dev, 2026-06-23): file CGM yang udah di-FREEZE
// (cognitive_handlers.go, cognitive_tensions.go, web/tabs/cognitive.js) TIDAK BOLEH dibuka
// lagi buat nambah filtur. SEMUA switch/tuning/hook CGM masuk SINI. File frozen cuma MANGGIL
// fungsi di file ini → ga pernah disentuh lagi. Ini jalan Flowork BEREVOLUSI tanpa unfreeze.
//
// 📖 WAJIB BACA: /home/mrflow/Documents/FLowork_os/lock/CognitiveGraph.md sebelum ngutak-atik
// (cara kerja graph, orphan/limit, tension/kontradiksi, tools, cara nambah filtur).
package agentmgr

import (
	"os"
	"strconv"
	"strings"
)

// cgmEnvInt — baca int dari env, fallback default kalau kosong/invalid/<=0.
func cgmEnvInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// cgmNodeLimit — DEFAULT jumlah node yg di-load ke viz CGM. Dinaikin dari 500→3000 (owner
// 2026-06-23): node "hub" (brain-root/hub-coding-instinct/dst) hit_count rendah → ke-drop
// dari top-N → ratusan instinct keliatan ORPHAN padahal nyambung. 3000 nutup graph penuh
// (~2241 node sekarang). Override TANPA unfreeze: env FLOWORK_CGM_NODE_LIMIT.
func cgmNodeLimit() int { return cgmEnvInt("FLOWORK_CGM_NODE_LIMIT", 3000) }

// cgmEdgeLimit — DEFAULT jumlah edge yg di-load. Dinaikin 1000→6000: ada 2324 edge, limit
// 1000 (strength DESC) buang 1324 edge (mis. instinct member_of strength rendah) → kabel
// putus di viz. 6000 nutup semua + ruang tumbuh. Override: env FLOWORK_CGM_EDGE_LIMIT.
func cgmEdgeLimit() int { return cgmEnvInt("FLOWORK_CGM_EDGE_LIMIT", 6000) }
