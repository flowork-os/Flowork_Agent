// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"

	"flowork-gui/internal/httpx"
)

var SchedulerFireFunc func(agentID, scheduleID string) (int64, error)

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

func SchedulerTriggerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))

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
