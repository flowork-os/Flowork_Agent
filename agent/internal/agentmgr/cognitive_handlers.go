// 🔒 FROZEN COGNITIVE-GRAPH · Repo: https://github.com/flowork-os/Flowork-OS · Owner: Aola Sahidin (Mr.Dev)
// ⛔ WAJIB sebelum ngedit: BACA /home/mrflow/Documents/FLowork_os/lock/CognitiveGraph.md
//    (cara kerja, orphan/limit, kontradiksi, tools, SWITCH). File BEKU (chattr +i + hash). Filtur baru →
//    CABANG internal/agentmgr/cognitive_ext.go (switch limit) / FILE BARU cognitive_<nama>.go / data.
//    JANGAN buka file beku ini.
//
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (autonomy grant 2026-06-19).
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-19 · FROZEN 2026-06-23 (limit switch via cognitive_ext.go, owner-approved)
// Reason: CGM read API handlers (graph/tensions) — built + unit-tested (build/vet/test green). Extend = cabang/file baru.
//
// cognitive_handlers.go — HTTP read API buat Cognitive Graph (CGM) per-agent.
//
// GUI tab "Cognitive Graph" (reuse pola codemap D3 force-graph) baca dari sini.
// Pola sama persis CodemapNodesHandler: validasi id → openAgentStore → query → JSON.
// Read-only; CRUD/edit graph nyusul kalau perlu (additive, plug-and-play).

package agentmgr

import (
	"net/http"
	"strings"

	"flowork-gui/internal/httpx"
)

// CognitiveGraphHandler — GET /api/agents/cognitive/graph?id=<agent>&limit=N
// → {nodes:[...], edges:[...]} buat divisualisasi (bola-bola nyambung).
func CognitiveGraphHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	// DEFAULT limit dari CABANG non-frozen cognitive_ext.go (cgmNodeLimit/cgmEdgeLimit) —
	// dinaikin biar node "hub" hit-rendah + edge instinct member_of GA ke-drop (anti
	// orphan-palsu). Override via env FLOWORK_CGM_NODE_LIMIT/EDGE_LIMIT TANPA buka file ini.
	nodes, err := store.ListCogNodes(parseLimitOr(r.URL.Query().Get("limit"), cgmNodeLimit()))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	edges, err := store.ListCogEdges(parseLimitOr(r.URL.Query().Get("edge_limit"), cgmEdgeLimit()))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"nodes": nodes, "edges": edges,
		"node_count": len(nodes), "edge_count": len(edges),
	})
}

// CognitiveTensionsHandler — GET /api/agents/cognitive/tensions?id=<agent>
// → kontradiksi 'open' yang nunggu owner putusin.
func CognitiveTensionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	items, err := store.ListOpenTensions(parseLimitOr(r.URL.Query().Get("limit"), 50))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}
