// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/httpx"
)

func ApprovalQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListApprovalQueue(status, parseLimitOr(r.URL.Query().Get("limit"), 100))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

func ApproveHandler(w http.ResponseWriter, r *http.Request) {
	decideApproval(w, r, "approved")
}

func RejectHandler(w http.ResponseWriter, r *http.Request) {
	decideApproval(w, r, "rejected")
}

func decideApproval(w http.ResponseWriter, r *http.Request, status string) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	queueID, _ := strconv.ParseInt(r.URL.Query().Get("queue_id"), 10, 64)
	decidedBy := r.Header.Get("X-Decided-By")
	if decidedBy == "" {
		decidedBy = "owner"
	}
	if agentID == "" || queueID == 0 {
		httpx.WriteJSON(w, map[string]any{"error": "id + queue_id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if err := store.DecideApproval(queueID, status, decidedBy); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":       true,
		"queue_id": queueID,
		"status":   status,
	})
}

func ToolAuditHandler(w http.ResponseWriter, r *http.Request) {
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
	rows, err := store.ListToolAudit(
		r.URL.Query().Get("decision"),
		r.URL.Query().Get("tool"),
		parseLimitOr(r.URL.Query().Get("limit"), 100),
	)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}
