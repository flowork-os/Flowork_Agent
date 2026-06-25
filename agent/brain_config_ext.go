// brain_config_ext.go — GROWTH-POINT (NON-frozen). GUI #4: per-agent "Agent Brain"
// curation = SUMBER KEBENARAN buat (a) scope insting per-peran (#3 RI-5) + (b) toggle
// defer/all-tools per-agent (#2C). Dulu cuma ENV global; sekarang GUI/kv per-agent.
//
// SHARED STORE: ~/.flowork/agent_brain_config.json — DIBACA dua proses:
//   - host (file ini): RegisterDeferPolicy → defer/all-tools per-agent.
//   - router (instinctenrich_ext2.go): instinct_domains per-agent buat scope.
//   Satu file = satu sumber kebenaran, GUI = penulis. Path via UserHomeDir → multi-OS.
//
// FAILS-SAFE: file ga ada / rusak / agent ga ke-set → fallback ENV (byte-identik perilaku
// lama). Additive total. Switch: tetep bisa ENV `FLOWORK_DEFER_TOOLS`/`FLOWORK_EXPOSE_ALL_TOOLS`
// + `FLOWORK_INSTINCT_SCOPE_MAP` kalau file kosong.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"flowork-gui/internal/agentmgr"
)

// agentBrainCfg — config per-agent (semua opsional; nil/empty = fallback ENV/default).
type agentBrainCfg struct {
	InstinctDomains []string `json:"instinct_domains,omitempty"` // Room ekstra (di luar baseline universal/tool), mis. ["instinct_coding"]
	DeferTools      *bool    `json:"defer_tools,omitempty"`      // nil = fallback ENV
	ExposeAll       *bool    `json:"expose_all,omitempty"`       // nil = fallback ENV
}

var brainCfgMu sync.Mutex

// brainConfigPath — ~/.flowork/agent_brain_config.json (DI LUAR repo = data user, ga ke-push).
func brainConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".flowork", "agent_brain_config.json")
}

func brainConfigLoad() map[string]agentBrainCfg {
	out := map[string]agentBrainCfg{}
	p := brainConfigPath()
	if p == "" {
		return out
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func brainConfigSave(m map[string]agentBrainCfg) error {
	p := brainConfigPath()
	if p == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// envDeferOn / envExposeAll — replikasi PERSIS tool_specs_defer.go (FROZEN) supaya fallback
// = byte-identik pas agent ga ke-set di file. JANGAN ubah daftar nilai tanpa samain ke sana.
func envDeferOn() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("FLOWORK_DEFER_TOOLS"))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}
func envExposeAll() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("FLOWORK_EXPOSE_ALL_TOOLS"))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// brainConfigDeferPolicy — hook RegisterDeferPolicy: per-agent dari file, else ENV (scoped-primary).
func brainConfigDeferPolicy(agentID string, isPrimary bool) (deferOn, exposeAll bool) {
	deferOn = envDeferOn() && isPrimary
	exposeAll = envExposeAll()
	brainCfgMu.Lock()
	cfg, ok := brainConfigLoad()[agentID]
	brainCfgMu.Unlock()
	if ok {
		if cfg.DeferTools != nil {
			deferOn = *cfg.DeferTools
		}
		if cfg.ExposeAll != nil {
			exposeAll = *cfg.ExposeAll
		}
	}
	return deferOn, exposeAll
}

// brainConfigHandler — GET ?id=<agent> → config agent itu (+ default); POST {agent,...} → set.
func brainConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		brainCfgMu.Lock()
		all := brainConfigLoad()
		brainCfgMu.Unlock()
		if id != "" {
			tfWriteJSON(w, 0, map[string]any{"agent": id, "config": all[id]})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"configs": all})
	case http.MethodPost:
		var body struct {
			Agent           string   `json:"agent"`
			InstinctDomains []string `json:"instinct_domains"`
			DeferTools      *bool    `json:"defer_tools"`
			ExposeAll       *bool    `json:"expose_all"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Agent) == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "need {agent,...}"})
			return
		}
		brainCfgMu.Lock()
		defer brainCfgMu.Unlock()
		m := brainConfigLoad()
		m[body.Agent] = agentBrainCfg{InstinctDomains: body.InstinctDomains, DeferTools: body.DeferTools, ExposeAll: body.ExposeAll}
		if err := brainConfigSave(m); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "agent": body.Agent, "config": m[body.Agent]})
	default:
		tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET/POST only"})
	}
}

func init() {
	// WIRE: pasang defer-policy per-agent (sumber tunggal resolveDeferPolicy). Fallback ENV → aman.
	RegisterFeature(Feature{Name: "brain-config-wire", Phase: PhaseWire, Apply: func(d *Deps) {
		agentmgr.RegisterDeferPolicy(brainConfigDeferPolicy)
	}})
	// ROUTE: endpoint kurasi GUI.
	RegisterFeature(Feature{Name: "brain-config-route", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/agents/brain-config", brainConfigHandler)
	}})
}
