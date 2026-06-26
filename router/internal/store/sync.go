// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"time"
)

type DispatchSettings struct {
	DefaultModel     string        `json:"defaultModel"`
	FallbackStrategy string        `json:"fallbackStrategy"`
	RtkTokenSaver    bool          `json:"rtkTokenSaver"`
	IntentRouting    IntentRouting `json:"intentRouting"`
	Budget           Budget        `json:"budget"`
}

type SyncBundle struct {
	Version        string               `json:"version"`
	ExportedAt     string               `json:"exportedAt"`
	Providers      []ProviderConnection `json:"providers,omitempty"`
	Combos         []Combo              `json:"combos,omitempty"`
	ProxyPools     []ProxyPool          `json:"proxyPools,omitempty"`
	Skills         []Skill              `json:"skills,omitempty"`
	MediaProviders []MediaProvider      `json:"mediaProviders,omitempty"`
	MCPServers     []MCPServer          `json:"mcpServers,omitempty"`
	Pricing        []Pricing            `json:"pricing,omitempty"`
	Tags           []Tag                `json:"tags,omitempty"`
	ModelAliases   []ModelAlias         `json:"modelAliases,omitempty"`
	CustomModels   []ModelCustom        `json:"customModels,omitempty"`
	Dispatch       *DispatchSettings    `json:"dispatch,omitempty"`
}

func ExportConfig(d *sql.DB) *SyncBundle {
	b := &SyncBundle{Version: "1", ExportedAt: time.Now().UTC().Format(time.RFC3339)}
	b.Providers, _ = ListProviders(d)
	b.Combos, _ = ListCombos(d)
	b.ProxyPools, _ = ListProxyPools(d)
	b.Skills, _ = ListSkills(d)
	b.MediaProviders, _ = ListMediaProviders(d, "")
	b.MCPServers, _ = ListMCPServers(d)
	b.Pricing, _ = ListPricing(d, "")
	b.Tags, _ = ListTags(d)
	b.ModelAliases, _ = ListModelAliases(d)
	b.CustomModels, _ = ListCustomModels(d)
	if s, err := LoadSettings(d); err == nil && s != nil {
		b.Dispatch = &DispatchSettings{
			DefaultModel: s.DefaultModel, FallbackStrategy: s.FallbackStrategy,
			RtkTokenSaver: s.RtkTokenSaver, IntentRouting: s.IntentRouting, Budget: s.Budget,
		}
	}
	return b
}

func ImportConfig(d *sql.DB, b *SyncBundle) map[string]int {
	n := map[string]int{}
	for i := range b.Providers {
		if UpsertProvider(d, &b.Providers[i]) == nil {
			n["providers"]++
		}
	}
	for i := range b.Combos {
		if UpsertCombo(d, &b.Combos[i]) == nil {
			n["combos"]++
		}
	}
	for i := range b.ProxyPools {
		if UpsertProxyPool(d, &b.ProxyPools[i]) == nil {
			n["proxyPools"]++
		}
	}
	for i := range b.Skills {
		if UpsertSkill(d, &b.Skills[i]) == nil {
			n["skills"]++
		}
	}
	for i := range b.MediaProviders {
		if UpsertMediaProvider(d, &b.MediaProviders[i]) == nil {
			n["mediaProviders"]++
		}
	}
	for i := range b.MCPServers {
		if UpsertMCPServer(d, &b.MCPServers[i]) == nil {
			n["mcpServers"]++
		}
	}
	for i := range b.Pricing {
		if UpsertPricing(d, &b.Pricing[i]) == nil {
			n["pricing"]++
		}
	}
	for i := range b.Tags {
		if UpsertTag(d, &b.Tags[i]) == nil {
			n["tags"]++
		}
	}
	for i := range b.ModelAliases {
		if UpsertModelAlias(d, &b.ModelAliases[i]) == nil {
			n["modelAliases"]++
		}
	}
	for i := range b.CustomModels {
		if UpsertCustomModel(d, &b.CustomModels[i]) == nil {
			n["customModels"]++
		}
	}

	if b.Dispatch != nil {
		if s, err := LoadSettings(d); err == nil && s != nil {
			s.DefaultModel = b.Dispatch.DefaultModel
			s.FallbackStrategy = b.Dispatch.FallbackStrategy
			s.RtkTokenSaver = b.Dispatch.RtkTokenSaver
			s.IntentRouting = b.Dispatch.IntentRouting
			s.Budget = b.Dispatch.Budget
			if SaveSettings(d, s) == nil {
				n["dispatch"]++
			}
		}
	}
	return n
}
