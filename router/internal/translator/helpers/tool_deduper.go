// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import "regexp"

type toolDedupPattern struct {
	literal string
	re      *regexp.Regexp
}

func litPattern(s string) toolDedupPattern { return toolDedupPattern{literal: s} }
func rePattern(p string) toolDedupPattern {
	return toolDedupPattern{re: regexp.MustCompile(p)}
}

type toolDedupRule struct {
	Triggers []toolDedupPattern
	Strip    []toolDedupPattern
}

var dedupRules = []toolDedupRule{
	{
		Triggers: []toolDedupPattern{
			litPattern("mcp__exa__web_search_exa"),
			litPattern("mcp__exa__web_fetch_exa"),
		},
		Strip: []toolDedupPattern{
			litPattern("WebSearch"),
			litPattern("WebFetch"),
			litPattern("mcp__workspace__web_fetch"),
		},
	},
	{
		Triggers: []toolDedupPattern{
			litPattern("mcp__tavily__tavily_search"),
			litPattern("mcp__tavily__tavily_extract"),
		},
		Strip: []toolDedupPattern{
			litPattern("WebSearch"),
			litPattern("WebFetch"),
			litPattern("mcp__workspace__web_fetch"),
		},
	},
	{
		Triggers: []toolDedupPattern{
			rePattern(`^mcp__browsermcp__`),
		},
		Strip: []toolDedupPattern{
			rePattern(`^mcp__Claude_in_Chrome__`),
		},
	},
}

func patternMatches(name string, pat toolDedupPattern) bool {
	if pat.re != nil {
		return pat.re.MatchString(name)
	}
	return pat.literal != "" && name == pat.literal
}

func extractToolName(t any) string {
	tool, ok := t.(map[string]any)
	if !ok {
		return ""
	}
	if n, _ := tool["name"].(string); n != "" {
		return n
	}
	if fn, ok := tool["function"].(map[string]any); ok {
		if n, _ := fn["name"].(string); n != "" {
			return n
		}
	}
	return ""
}

func DedupeTools(tools []any) (out []any, stripped []string) {
	if len(tools) == 0 {
		return tools, nil
	}
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = extractToolName(t)
	}

	strip := map[string]bool{}
	for _, rule := range dedupRules {
		hasTrigger := false
		for _, n := range names {
			if n == "" {
				continue
			}
			for _, t := range rule.Triggers {
				if patternMatches(n, t) {
					hasTrigger = true
					break
				}
			}
			if hasTrigger {
				break
			}
		}
		if !hasTrigger {
			continue
		}
		for _, n := range names {
			if n == "" {
				continue
			}
			for _, s := range rule.Strip {
				if patternMatches(n, s) {
					strip[n] = true
					break
				}
			}
		}
	}

	if len(strip) == 0 {
		return tools, nil
	}
	out = make([]any, 0, len(tools))
	for i, t := range tools {
		if strip[names[i]] {
			continue
		}
		out = append(out, t)
	}
	stripped = make([]string, 0, len(strip))
	for n := range strip {
		stripped = append(stripped, n)
	}
	return out, stripped
}
