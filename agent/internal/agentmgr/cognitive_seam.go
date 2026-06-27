// 📄 Dok: FLowork_os/lock/CognitiveGraph.md
//
// cognitive_seam.go — SWITCH-READER BEKU (POLA-B) buat CGM core (cognitive_handlers.go frozen).
// Pisahan dari cognitive_ext.go (non-frozen, titik extension): switch-reader + default aman ADA
// DI SINI biar inti beku self-sufficient (delete-test §6.4: hapus _ext → fungsi ini tetep ada,
// build OK). Switch sejati = ENV (FLOWORK_CGM_*), jadi bekuin pembacanya TIDAK ngurangin tuning.
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

// cgmNodeLimit — DEFAULT node di-load ke viz CGM (3000, nutup ~2241 node penuh). Override TANPA
// unfreeze: env FLOWORK_CGM_NODE_LIMIT.
func cgmNodeLimit() int { return cgmEnvInt("FLOWORK_CGM_NODE_LIMIT", 3000) }

// cgmEdgeLimit — DEFAULT edge di-load (6000, nutup ~2324 edge + ruang tumbuh). Override: env
// FLOWORK_CGM_EDGE_LIMIT.
func cgmEdgeLimit() int { return cgmEnvInt("FLOWORK_CGM_EDGE_LIMIT", 6000) }
