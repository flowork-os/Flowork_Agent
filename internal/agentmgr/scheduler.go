// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// 2026-06-11 (owner-approved security audit, unfreeze→refreeze): loopback-secret
//   check now uses subtle.ConstantTimeCompare (matches loket/service.go and
//   triggers/engine.go) instead of ==.
// Reason: Section 18 phase 1 admin endpoints. API stable:
//   GET  /api/agents/scheduler/runs?id=<agent>&schedule=&limit=
//   POST /api/agents/scheduler/trigger?id=<agent>&schedule_id=
// Phase 2 (UI integration, real-time stream) → tambah file baru.
//
// scheduler.go — Section 18 admin: list runs + manual trigger.

package agentmgr

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"

	"flowork-gui/internal/httpx"
)

// SchedulerFireFunc — callback dari main.go untuk panggil engine.FireNow.
// Lazy-bind di main wiring supaya scheduler engine instance dependency
// ngga circular.
var SchedulerFireFunc func(agentID, scheduleID string) (int64, error)

// SchedulerRunsHandler — GET /api/agents/scheduler/runs?id=&schedule=&limit=
func SchedulerRunsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule"))
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if err := store.SchedulerSchemaInit(); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	runs, err := store.ListSchedulerRuns(scheduleID, limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"items": runs,
		"count": len(runs),
	})
}

// SchedulerTriggerHandler — POST /api/agents/scheduler/trigger?id=&schedule_id=
func SchedulerTriggerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	// Caller-bound execution: when reached over the loopback self-API (secret
	// present), the VERIFIED caller id is authoritative — an agent cannot trigger
	// ANOTHER agent's schedule by passing ?id=<other>. Same guard ToolRunHandler uses.
	if secret := strings.TrimSpace(os.Getenv("FLOWORK_LOOPBACK_SECRET")); secret != "" &&
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Flowork-Secret")), []byte(secret)) == 1 {
		if caller := strings.TrimSpace(r.Header.Get("X-Flowork-Caller")); caller != "" {
			if agentID != "" && agentID != caller {
				httpx.WriteJSON(w, map[string]any{"error": "agent identity mismatch (caller-bound execution)"})
				return
			}
			agentID = caller
		}
	}
	if !reID.MatchString(agentID) || scheduleID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "valid agent id + schedule_id required"})
		return
	}
	if SchedulerFireFunc == nil {
		httpx.WriteJSON(w, map[string]any{"error": "scheduler engine not wired"})
		return
	}
	runID, err := SchedulerFireFunc(agentID, scheduleID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":     true,
		"run_id": runID,
	})
}
