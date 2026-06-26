// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func promptIsPrivate(req OpenAIRequest, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	var sb strings.Builder
	for _, m := range req.Messages {
		if m.Role == "user" || m.Role == "system" {
			sb.WriteString(strings.ToLower(m.Content))
			sb.WriteByte('\n')
		}
	}
	text := sb.String()
	for _, p := range patterns {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" && strings.Contains(text, p) {
			return true
		}
	}
	return false
}

func filterByTag(matches []store.ProviderConnection, tag string) []store.ProviderConnection {
	tag = strings.ToLower(strings.TrimSpace(tag))
	var out []store.ProviderConnection
	for _, p := range matches {
		if providerHasTag(p, tag) {
			out = append(out, p)
		}
	}
	return out
}

func providerHasTag(p store.ProviderConnection, tag string) bool {
	tags, _ := p.Data["tags"].([]any)
	for _, t := range tags {
		if s, ok := t.(string); ok && strings.ToLower(s) == tag {
			return true
		}
	}
	return false
}
