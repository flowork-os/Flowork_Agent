// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package router

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

func init() { RegisterInstinctSelectorCtx(scopedInstinctSelector) }

func instinctScopedEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_INSTINCT_SCOPED"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

var baselineRooms = map[string]bool{
	"instinct_universal": true,
	"instinct_tool":      true,
}

var roleDomains = map[string]map[string]bool{
	"mr-flow": {"instinct_coding": true, "instinct_security": true, "instinct_crypto": true, "instinct_bisnis": true},
}

func scopeFromBrainConfig(agentID string) (map[string]bool, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, false
	}
	raw, err := os.ReadFile(filepath.Join(home, ".flowork", "agent_brain_config.json"))
	if err != nil {
		return nil, false
	}
	var all map[string]struct {
		InstinctDomains []string `json:"instinct_domains"`
	}
	if json.Unmarshal(raw, &all) != nil {
		return nil, false
	}
	cfg, ok := all[agentID]
	if !ok || len(cfg.InstinctDomains) == 0 {
		return nil, false
	}
	set := map[string]bool{}
	for _, d := range cfg.InstinctDomains {
		if d = strings.TrimSpace(d); d != "" {
			set[d] = true
		}
	}
	if len(set) == 0 {
		return nil, false
	}
	return set, true
}

func lookupDomains(agentID string) (map[string]bool, bool) {
	if set, ok := scopeFromBrainConfig(agentID); ok {
		return set, true
	}
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

func brainExternalScopeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_BRAIN_EXTERNAL_SCOPE"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

func externalScopedSelector(all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer {
	filtered := make([]brain.InstinctDrawer, 0, len(all))
	for _, d := range all {
		if strings.TrimSpace(d.Room) == "instinct_tool" {
			continue
		}
		filtered = append(filtered, d)
	}
	if len(filtered) == 0 {
		return semanticInstinctSelector(all, query, max)
	}
	log.Printf("flow_router instinct-scope: caller=EXTERNAL skip=instinct_tool %d→%d kandidat", len(all), len(filtered))
	return semanticInstinctSelector(filtered, query, max)
}

func scopedInstinctSelector(ctx context.Context, all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer {
	agentID := AgentIDFromContext(ctx)

	if agentID == "" && brainExternalScopeEnabled() {
		return externalScopedSelector(all, query, max)
	}
	if !instinctScopedEnabled() {
		return semanticInstinctSelector(all, query, max)
	}
	if agentID == "" {
		return semanticInstinctSelector(all, query, max)
	}
	domains, mapped := lookupDomains(agentID)
	if !mapped {
		return semanticInstinctSelector(all, query, max)
	}
	filtered := make([]brain.InstinctDrawer, 0, len(all))
	for _, d := range all {
		if room := strings.TrimSpace(d.Room); baselineRooms[room] || domains[room] {
			filtered = append(filtered, d)
		}
	}
	log.Printf("flow_router instinct-scope: agent=%q domains=%v %d→%d kandidat", agentID, keysOf(domains), len(all), len(filtered))
	if len(filtered) == 0 {
		return semanticInstinctSelector(all, query, max)
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
