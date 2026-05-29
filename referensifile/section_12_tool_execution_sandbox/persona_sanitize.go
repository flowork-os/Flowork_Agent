// Package tools — persona_sanitize.go: shared helper untuk normalize persona name.
//
// Sebelumnya ada di forum_post.go (deleted 2026-05-17 per Ayah deprecate
// forum + vote + roadmap_write). Plan tool masih perlu helper ini.

package tools

import (
	"regexp"
	"strings"
)

var personaSanitizeRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// sanitizePersonaName normalize persona/agent name ke lowercase + alnum + _-.
// Empty input return empty (caller handle fallback).
func sanitizePersonaName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = personaSanitizeRe.ReplaceAllString(s, "")
	return s
}
