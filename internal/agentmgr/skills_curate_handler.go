// skills_curate_handler.go — FASE 8: HTTP endpoint Curator skill (per-agent).
//
//	GET  /api/agents/skills?id=<agent>[&archived=1]  → list skill + grade
//	POST /api/agents/skills/curate?id=<agent>         → jalanin curator → report
//
// Curator = lifecycle skill: consolidate dup + arsip stale + grade. Logic di
// agentdb.CurateSkills (per-agent, isolated). Default idle 90d / umur 30d.

package agentmgr

import (
	"net/http"
	"strings"
	"time"

	"flowork-gui/internal/httpx"
)

// SkillsListHandler — GET list skill + grade.
func SkillsListHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		httpx.WriteJSON(w, map[string]any{"error": "id required"})
		return
	}
	store, err := openAgentStore(id)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	includeArchived := r.URL.Query().Get("archived") == "1"
	skills, err := store.ListSkillsGraded(includeArchived)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"id": id, "count": len(skills), "skills": skills})
}

// SkillsCurateHandler — POST jalanin curator buat 1 agent.
func SkillsCurateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "POST only"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		httpx.WriteJSON(w, map[string]any{"error": "id required"})
		return
	}
	store, err := openAgentStore(id)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rep, err := store.CurateSkills(time.Now().UTC(), 90, 30)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"id": id, "report": rep})
}

// CurateAllAgentsSkills — sapuan curator semua agent (dipanggil cron harian).
// agentIDs di-pass caller (host.AgentIDs). Best-effort per-agent.
func CurateAllAgentsSkills(agentIDs []string) map[string]any {
	out := map[string]any{}
	for _, id := range agentIDs {
		store, err := openAgentStore(id)
		if err != nil {
			continue
		}
		rep, cerr := store.CurateSkills(time.Now().UTC(), 90, 30)
		store.Close()
		if cerr == nil && (len(rep.Consolidated) > 0 || len(rep.Stale) > 0) {
			out[id] = rep
		}
	}
	return out
}
