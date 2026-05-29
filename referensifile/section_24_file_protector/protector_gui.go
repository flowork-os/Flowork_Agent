// protector.go — GUI API for managing Protected Core file/folder rules.
//
// Allows Ayah to add, remove, toggle, and test file protection rules
// via the dashboard UI at http://127.0.0.1:3101/ (tab: Sistem Imun → Protector).
//
// Hardcoded baseline rules (interceptors.go) cannot be deleted — only disabled.
// Custom rules are stored in state/protector/rules.json and merged at runtime.
package guiapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/tools"
)

// BUG-006 fix: mutex protects read-modify-write cycle on dynamic rules.
// Without this, concurrent add/remove/toggle requests can lose updates.
var protectorMu sync.Mutex

// protectorRule is the unified view of a rule (hardcoded or custom) for the UI.
type protectorRule struct {
	Path     string `json:"path"`
	Type     string `json:"type"`   // "basename" or "suffix"
	Source   string `json:"source"` // "hardcoded" or "custom"
	Active   bool   `json:"active"`
	Category string `json:"category"` // "secrets", "core", "doktrin", "entry", "docs", "config", "custom"
}

// ProtectorListHandler — GET /api/protector
// Returns unified list of all protection rules (hardcoded + custom).
func ProtectorListHandler(workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules := buildUnifiedRules()
		stats := map[string]int{
			"total":     len(rules),
			"hardcoded": 0,
			"custom":    0,
			"disabled":  0,
		}
		for _, r := range rules {
			if r.Source == "hardcoded" {
				stats["hardcoded"]++
			} else {
				stats["custom"]++
			}
			if !r.Active {
				stats["disabled"]++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"rules": rules,
			"stats": stats,
		})
	}
}

// ProtectorAddHandler — POST /api/protector/add {path, type, category}
func ProtectorAddHandler(workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Path     string `json:"path"`
			Type     string `json:"type"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			safeError(w, http.StatusBadRequest, "bad json", err)
			return
		}
		req.Path = strings.TrimSpace(req.Path)
		if req.Path == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}
		if req.Type != "basename" && req.Type != "suffix" {
			req.Type = "suffix"
		}
		if req.Category == "" {
			req.Category = "custom"
		}

		protectorMu.Lock()
		defer protectorMu.Unlock()
		st := tools.GetDynamicState()
		// Check for duplicate
		for _, rule := range st.Rules {
			if strings.EqualFold(rule.Path, req.Path) {
				http.Error(w, "rule already exists", http.StatusConflict)
				return
			}
		}
		st.Rules = append(st.Rules, tools.DynamicRule{
			Path:     req.Path,
			Type:     req.Type,
			Active:   true,
			Category: req.Category,
		})
		st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		st.UpdatedBy = "ayah-gui"

		if err := tools.SaveDynamicRules(workspace, &st); err != nil {
			safeError(w, http.StatusInternalServerError, "save rules failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Rule added", "path": req.Path})
	}
}

// ProtectorRemoveHandler — POST /api/protector/remove {path}
// Only custom rules can be removed. Hardcoded rules return error.
func ProtectorRemoveHandler(workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			safeError(w, http.StatusBadRequest, "bad json", err)
			return
		}
		req.Path = strings.TrimSpace(req.Path)

		// Check if hardcoded — cannot delete, only disable via toggle
		if isHardcodedRule(req.Path) {
			http.Error(w, "cannot delete hardcoded rule — use toggle to disable", http.StatusForbidden)
			return
		}

		protectorMu.Lock()
		defer protectorMu.Unlock()
		st := tools.GetDynamicState()
		found := false
		filtered := make([]tools.DynamicRule, 0, len(st.Rules))
		for _, rule := range st.Rules {
			if strings.EqualFold(rule.Path, req.Path) {
				found = true
				continue
			}
			filtered = append(filtered, rule)
		}
		if !found {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		st.Rules = filtered
		st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		st.UpdatedBy = "ayah-gui"

		if err := tools.SaveDynamicRules(workspace, &st); err != nil {
			safeError(w, http.StatusInternalServerError, "save rules failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Rule removed", "path": req.Path})
	}
}

// ProtectorToggleHandler — POST /api/protector/toggle {path, active}
// Works for both hardcoded and custom rules.
func ProtectorToggleHandler(workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Path   string `json:"path"`
			Active bool   `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			safeError(w, http.StatusBadRequest, "bad json", err)
			return
		}
		req.Path = strings.TrimSpace(req.Path)

		protectorMu.Lock()
		defer protectorMu.Unlock()
		st := tools.GetDynamicState()

		if isHardcodedRule(req.Path) {
			// Toggle hardcoded: add/remove from disabled list
			if req.Active {
				// Re-enable: remove from disabled list
				filtered := make([]string, 0, len(st.DisabledHardcoded))
				for _, d := range st.DisabledHardcoded {
					if !strings.EqualFold(d, req.Path) {
						filtered = append(filtered, d)
					}
				}
				st.DisabledHardcoded = filtered
			} else {
				// Disable: add to disabled list (if not already)
				found := false
				for _, d := range st.DisabledHardcoded {
					if strings.EqualFold(d, req.Path) {
						found = true
						break
					}
				}
				if !found {
					st.DisabledHardcoded = append(st.DisabledHardcoded, req.Path)
				}
			}
		} else {
			// Toggle custom rule
			for i, rule := range st.Rules {
				if strings.EqualFold(rule.Path, req.Path) {
					st.Rules[i].Active = req.Active
					break
				}
			}
		}

		st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		st.UpdatedBy = "ayah-gui"

		if err := tools.SaveDynamicRules(workspace, &st); err != nil {
			safeError(w, http.StatusInternalServerError, "save rules failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "path": req.Path, "active": req.Active,
		})
	}
}

// ProtectorTestHandler — POST /api/protector/test {path: "xxx"}
// Bug #7 fix: changed from GET to POST to prevent unauthenticated info
// disclosure about protection blind spots (GET bypasses RequireOwner).
func ProtectorTestHandler(workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			safeError(w, http.StatusBadRequest, "bad json", err)
			return
		}
		p := strings.TrimSpace(req.Path)
		if p == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}
		protected := tools.IsSensitivePath(workspace, p)
		writeJSON(w, http.StatusOK, map[string]any{
			"path":      p,
			"protected": protected,
		})
	}
}

// ── helpers ──

func isHardcodedRule(path string) bool {
	low := strings.ToLower(path)
	for k := range tools.HardcodedBasenames() {
		if strings.ToLower(k) == low {
			return true
		}
	}
	for _, s := range tools.HardcodedSuffixes() {
		if strings.ToLower(s) == low {
			return true
		}
	}
	return false
}

func buildUnifiedRules() []protectorRule {
	var rules []protectorRule
	dynState := tools.GetDynamicState()
	disabledSet := map[string]bool{}
	for _, d := range dynState.DisabledHardcoded {
		disabledSet[strings.ToLower(d)] = true
	}

	// Hardcoded basenames
	for k := range tools.HardcodedBasenames() {
		rules = append(rules, protectorRule{
			Path:     k,
			Type:     "basename",
			Source:   "hardcoded",
			Active:   !disabledSet[strings.ToLower(k)],
			Category: categorizeHardcoded(k),
		})
	}
	// Hardcoded suffixes
	for _, s := range tools.HardcodedSuffixes() {
		rules = append(rules, protectorRule{
			Path:     s,
			Type:     "suffix",
			Source:   "hardcoded",
			Active:   !disabledSet[strings.ToLower(s)],
			Category: categorizeHardcoded(s),
		})
	}
	// Dynamic custom rules
	for _, r := range dynState.Rules {
		rules = append(rules, protectorRule{
			Path:     r.Path,
			Type:     r.Type,
			Source:   "custom",
			Active:   r.Active,
			Category: r.Category,
		})
	}
	return rules
}

func categorizeHardcoded(path string) string {
	low := strings.ToLower(path)
	switch {
	case strings.Contains(low, ".env") || low == "owner.hash" || strings.Contains(low, "keys/") || strings.Contains(low, "sessions/"):
		return "secrets"
	case strings.Contains(low, "internal/core/") || strings.Contains(low, "internal/tools/") || strings.Contains(low, "internal/mesh/"):
		return "core"
	case strings.Contains(low, "promp/") || strings.Contains(low, "/promp/") || low == "agents.md" || low == "flow.md" || low == "claude.md":
		return "doktrin"
	case strings.HasPrefix(low, "cmd/"):
		return "entry"
	case strings.Contains(low, "docs/"):
		return "docs"
	case strings.Contains(low, "internal/provider/") || strings.Contains(low, "internal/session/") || strings.Contains(low, "internal/compact/"):
		return "infra"
	case strings.Contains(low, "internal/ownerauth/") || strings.Contains(low, "internal/selfupdate/") || strings.Contains(low, "internal/coremanifest/"):
		return "security"
	case low == "go.mod" || low == "go.sum" || strings.Contains(low, "config.yaml") || strings.Contains(low, ".mcp.json") || strings.Contains(low, "settings"):
		return "config"
	default:
		return "other"
	}
}
