// instinctenrich_ext2.go — GROWTH-POINT (NON-frozen). #3 scoped-instinct (RI-5).
//
// AKAR: insting lintas-domain ke-inject ke SIAPA AJA (coder dapet insting bisnis dst) →
// noise + boros token. Scope per-peran: tiap agent cuma dapet insting domain-nya (+ baseline
// universal/tool yg WAJIB buat semua). Selector ctx-aware (instinctSelectHookCtx) baca caller
// agent id (X-Agent-ID → ctx, lihat agentctx_ext.go) → filter by Room (= where_domain).
//
// 🔌 SWITCH: ENV FLOWORK_INSTINCT_SCOPED=1 (default OFF → delegasi ke selector semantic lama,
//    NOL perubahan perilaku). _ext2 di-init SETELAH _ext.go → RegisterInstinctSelectorCtx
//    MENANG atas RegisterInstinctSelector (lihat call-site instinctenrich.go).
//
// 🔧 ROLE-MAP tunable RUNTIME (tanpa rebuild): ENV FLOWORK_INSTINCT_SCOPE_MAP =
//    "agent-id:instinct_coding,instinct_security;agent2:instinct_bisnis" → di-merge DI ATAS
//    map compiled di bawah. Owner atur scope per agent dari flowork.local.env.
//
// FAILS-OPEN total: switch off / agent ga ke-map / hasil filter kosong / agent id kosong
//    (external / agent belum di-rebuild kirim X-Agent-ID) → balik ke semanticInstinctSelector
//    (perilaku proven). Insting BASELINE (universal+tool) SELALU lolos → ga ada agent "buta tool".
package router

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

func init() { RegisterInstinctSelectorCtx(scopedInstinctSelector) }

// instinctScopedEnabled — master switch #3. Default OFF (perilaku lama).
func instinctScopedEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_INSTINCT_SCOPED"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

// baselineRooms — insting WAJIB buat SEMUA agent (anti-starvation): reasoning universal +
// kesadaran tool (#2C). Selalu lolos filter, apapun perannya.
var baselineRooms = map[string]bool{
	"instinct_universal": true,
	"instinct_tool":      true,
}

// roleDomains — role-registry COMPILED (default): agent-id → set Room domain EKSTRA (di luar
// baseline). Agent yg TIDAK ada di sini DAN ga ada di ENV = TIDAK di-scope (fails-open).
// mr-flow = orchestrator generalis → SENGAJA semua domain (scoping no-op aman + contoh format).
var roleDomains = map[string]map[string]bool{
	"mr-flow": {"instinct_coding": true, "instinct_security": true, "instinct_crypto": true, "instinct_bisnis": true},
}

// lookupDomains — domain peran efektif buat agentID: ENV override (FLOWORK_INSTINCT_SCOPE_MAP)
// menang atas compiled. Return (set, true) kalau agent ke-scope; (nil, false) = fails-open.
func lookupDomains(agentID string) (map[string]bool, bool) {
	if raw := strings.TrimSpace(os.Getenv("FLOWORK_INSTINCT_SCOPE_MAP")); raw != "" {
		for _, entry := range strings.Split(raw, ";") {
			kv := strings.SplitN(strings.TrimSpace(entry), ":", 2)
			if len(kv) != 2 || strings.TrimSpace(kv[0]) != agentID {
				continue
			}
			set := map[string]bool{}
			for _, r := range strings.Split(kv[1], ",") {
				if r = strings.TrimSpace(r); r != "" {
					set[r] = true
				}
			}
			return set, true
		}
	}
	d, ok := roleDomains[agentID]
	return d, ok
}

// scopedInstinctSelector — selektor ctx-aware #3. Filter insting by domain-peran, lalu rank
// pakai semanticInstinctSelector (reuse RI-1 vindex). Fails-open di tiap titik.
func scopedInstinctSelector(ctx context.Context, all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer {
	if !instinctScopedEnabled() {
		return semanticInstinctSelector(all, query, max) // switch off → perilaku PROVEN
	}
	agentID := AgentIDFromContext(ctx)
	if agentID == "" {
		return semanticInstinctSelector(all, query, max) // anonim/external/belum-rebuild → fails-open
	}
	domains, mapped := lookupDomains(agentID)
	if !mapped {
		return semanticInstinctSelector(all, query, max) // agent belum di-scope → fails-open
	}
	filtered := make([]brain.InstinctDrawer, 0, len(all))
	for _, d := range all {
		if room := strings.TrimSpace(d.Room); baselineRooms[room] || domains[room] {
			filtered = append(filtered, d)
		}
	}
	log.Printf("flow_router instinct-scope: agent=%q domains=%v %d→%d kandidat", agentID, keysOf(domains), len(all), len(filtered))
	if len(filtered) == 0 {
		return semanticInstinctSelector(all, query, max) // jaga2 jangan starve
	}
	return semanticInstinctSelector(filtered, query, max)
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, strings.TrimPrefix(k, "instinct_"))
	}
	return out
}
