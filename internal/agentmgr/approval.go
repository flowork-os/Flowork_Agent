// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 3 approval endpoints + tool_audit query.
//
// approval.go — Section 12 phase 3: approval workflow + tool_audit endpoints.

package agentmgr

import (
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/httpx"
)

// ApprovalQueueHandler — GET/POST /api/agents/protector/approval/queue?id=<agent>
//   GET ?status=pending → list
//   (POST handled by separate approve/reject endpoints)
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

// ApproveHandler — POST /api/agents/protector/approve_pending?id=&queue_id=
func ApproveHandler(w http.ResponseWriter, r *http.Request) {
	decideApproval(w, r, "approved")
}

// RejectHandler — POST /api/agents/protector/reject_pending?id=&queue_id=
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

// ToolAuditHandler — GET /api/agents/tool-audit?id=&decision=&tool=&limit=
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
