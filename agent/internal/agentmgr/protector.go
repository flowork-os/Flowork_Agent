// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/protector"
)

func ProtectorRulesHandler(w http.ResponseWriter, r *http.Request) {
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
	switch r.Method {
	case http.MethodGet:
		custom, err := store.ListProtectorRules()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		out := []map[string]any{}

		for _, r := range custom {
			out = append(out, map[string]any{
				"id":         r.ID,
				"rule_type":  r.RuleType,
				"pattern":    r.Pattern,
				"action":     r.Action,
				"source":     r.Source,
				"enabled":    r.Enabled,
				"created_at": r.CreatedAt,
			})
		}
		if r.URL.Query().Get("include_baseline") == "1" {
			for _, b := range protector.Baseline() {
				out = append(out, map[string]any{
					"id":        0,
					"rule_type": b.Type,
					"pattern":   b.Pattern,
					"action":    b.Action,
					"source":    protector.SourceHardcoded,
					"enabled":   true,
					"immutable": true,
				})
			}
		}
		httpx.WriteJSON(w, map[string]any{"items": out, "count": len(out)})
	case http.MethodPost:
		var body agentdb.ProtectorRule
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		id, err := store.AddProtectorRule(body)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("rule_id"), 10, 64)
		if id == 0 {
			httpx.WriteJSON(w, map[string]any{"error": "rule_id required"})
			return
		}
		if err := store.DeleteProtectorRule(id); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

func ProtectorTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	var body struct {
		RuleType  string `json:"rule_type"`
		Candidate string `json:"candidate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}

	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	customDB, _ := store.ListProtectorRules()
	custom := []protector.BaselineRule{}
	for _, c := range customDB {
		if !c.Enabled {
			continue
		}
		custom = append(custom, protector.BaselineRule{
			Type: c.RuleType, Pattern: c.Pattern, Action: c.Action,
		})
	}
	matched, hit := protector.CheckPattern(body.RuleType, body.Candidate, custom)
	resp := map[string]any{
		"hit":       hit,
		"candidate": body.Candidate,
		"rule_type": body.RuleType,
	}
	if hit {
		resp["pattern"] = matched.Pattern
		resp["action"] = matched.Action
	}
	httpx.WriteJSON(w, resp)
}

func ProtectorAuditHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
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
	rows, err := store.ListProtectorAudit(r.URL.Query().Get("from"), r.URL.Query().Get("to"), limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}
