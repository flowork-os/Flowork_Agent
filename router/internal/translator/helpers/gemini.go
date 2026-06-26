// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import (
	"regexp"
	"strings"
)

func MapGeminiRole(role string) string {
	switch role {
	case "assistant":
		return "model"
	case "system":
		return "user"
	}
	return role
}

func MapGeminiFinishReason(reason string) string {
	switch reason {
	case "STOP", "":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	}
	return "stop"
}

var reFunctionName = regexp.MustCompile(`[^a-zA-Z0-9_.:\-]`)

func CleanFunctionName(name string) string {
	if name == "" {
		return "_unknown"
	}
	s := reFunctionName.ReplaceAllString(name, "_")
	if s == "" {
		return "_unknown"
	}
	if !isAlphaOrUnderscore(s[0]) {
		s = "_" + s
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func isAlphaOrUnderscore(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func CleanJSONSchemaForAntigravity(node any) {
	switch m := node.(type) {
	case map[string]any:
		for _, k := range []string{"$schema", "additionalProperties", "definitions", "$defs", "title", "examples"} {
			delete(m, k)
		}

		if t, ok := m["type"].([]any); ok {
			pruned := make([]any, 0, len(t))
			for _, x := range t {
				if s, _ := x.(string); s != "" && s != "null" {
					pruned = append(pruned, s)
				}
			}
			if len(pruned) == 1 {
				m["type"] = pruned[0]
			} else if len(pruned) > 1 {
				m["type"] = pruned
			} else {
				delete(m, "type")
			}
		}
		for _, v := range m {
			CleanJSONSchemaForAntigravity(v)
		}
	case []any:
		for _, v := range m {
			CleanJSONSchemaForAntigravity(v)
		}
	}
	_ = strings.Builder{}
}
