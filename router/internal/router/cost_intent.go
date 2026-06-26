// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type CostTier string

const (
	TierCheap    CostTier = "cheap"
	TierStandard CostTier = "standard"
	TierStrong   CostTier = "strong"
)

func ClassifyCost(req OpenAIRequest, cfg store.CostRouting) CostTier {

	if cfg.StrongOnToolUse && requestHasToolUse(req) {
		return TierStrong
	}

	totalChars := 0
	hasCode := false
	for _, m := range req.Messages {
		totalChars += len(m.Content)
		if !hasCode && cfg.StrongOnCode && containsCodeBlock(m.Content) {
			hasCode = true
		}
	}
	if hasCode {
		return TierStrong
	}

	if cfg.StrongMinMessages > 0 && len(req.Messages) >= cfg.StrongMinMessages {
		return TierStrong
	}

	if cfg.StandardMaxChars > 0 && totalChars > cfg.StandardMaxChars {
		return TierStrong
	}
	if cfg.CheapMaxChars > 0 && totalChars > cfg.CheapMaxChars {
		return TierStandard
	}
	return TierCheap
}

func requestHasToolUse(req OpenAIRequest) bool {
	if len(req.Tools) > 2 {
		return true
	}
	for _, m := range req.Messages {
		if len(m.ToolCalls) > 2 {
			return true
		}
		if m.ToolCallID != "" {
			return true
		}
	}
	return false
}

func containsCodeBlock(s string) bool {
	return strings.Contains(s, "```")
}

func filterByTier(matches []store.ProviderConnection, tier CostTier) []store.ProviderConnection {
	tag := "tier:" + string(tier)
	var out []store.ProviderConnection
	for _, p := range matches {
		if providerHasTag(p, tag) {
			out = append(out, p)
		}
	}
	return out
}

func hasActiveProviderForModel(matches []store.ProviderConnection, model string) bool {
	if model == "" {
		return false
	}
	for _, p := range matches {
		models, _ := p.Data[store.CfgModels].([]any)
		for _, m := range models {
			if s, ok := m.(string); ok && s == model {
				return true
			}
		}
	}
	return false
}
