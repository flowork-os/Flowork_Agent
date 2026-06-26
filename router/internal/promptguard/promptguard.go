// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package promptguard

import (
	"regexp"
	"strings"
)

const AlgoVersion = "v1"

type Severity string

const (
	SeveritySafe       Severity = "safe"
	SeverityReview     Severity = "review"
	SeveritySuspicious Severity = "suspicious"
)

type Hit struct {
	Category string  `json:"category"`
	Pattern  string  `json:"pattern"`
	Snippet  string  `json:"snippet"`
	Weight   float64 `json:"weight"`
}

type Result struct {
	AlgoVersion        string   `json:"algo_version"`
	Allowed            bool     `json:"allowed"`
	Severity           Severity `json:"severity"`
	Score              float64  `json:"score"`
	Hits               []Hit    `json:"hits,omitempty"`
	EducationalContext bool     `json:"educational_context"`
	Reason             string   `json:"reason,omitempty"`
}

type signature struct {
	Category string
	Name     string
	Re       *regexp.Regexp
	Weight   float64
}

var signatures []signature

func init() {

	mustCompile := func(s string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)` + s)
	}

	signatures = []signature{

		{"instruction_override", "ignore_previous", mustCompile(`\bignore (the )?(previous|prior|above|all) (instructions?|prompts?|rules?|context|messages?)\b`), 0.5},
		{"instruction_override", "disregard_above", mustCompile(`\b(disregard|forget) (the )?(previous|prior|above|all) (instructions?|prompts?|rules?)\b`), 0.5},
		{"instruction_override", "override_directive", mustCompile(`\boverride (the )?(system|previous|all) (prompt|instruction|directive)\b`), 0.5},

		{"role_hijack", "you_are_now", mustCompile(`\byou are (now|actually) (?:a )?(jailbroken|uncensored|unfiltered|dan|stan|developer mode|admin|root|godmode)\b`), 0.4},
		{"role_hijack", "pretend_to_be", mustCompile(`\bpretend (?:to be|you are) (?:a )?(jailbroken|uncensored|evil|hacker|dan|stan)\b`), 0.3},
		{"role_hijack", "act_as", mustCompile(`\bact as (?:if you (?:are|were) )?(jailbroken|uncensored|evil|godmode|admin)\b`), 0.3},

		{"system_leak", "reveal_prompt", mustCompile(`\b(reveal|show|print|display|output) (your|the) (system )?prompt\b`), 0.5},

		{"system_leak", "system_prefix", mustCompile(`(?m)^(system|admin|root):\s`), 0.2},
		{"system_leak", "developer_mode", mustCompile(`\b(enable|activate|enter) (developer|debug|admin|god) mode\b`), 0.3},

		{"jailbreak", "dan_prompt", mustCompile(`\bDAN (mode|prompt|jailbreak)\b`), 0.4},
		{"jailbreak", "anything_now", mustCompile(`\b(do|say) anything now\b`), 0.4},
		{"jailbreak", "no_restriction", mustCompile(`\b(no|without|bypass) (restrictions?|limitations?|guidelines?|rules?|filter|filters)\b`), 0.3},
		{"jailbreak", "hypothetical_evil", mustCompile(`\bhypothetically(,)? (if you were|imagine you are) (evil|unaligned|jailbroken)\b`), 0.3},
	}
}

var educationalPrefixes = []string{
	"example:",
	"tutorial:",
	"explain ",
	"explanation:",
	"discuss ",
	"discussion:",
	"in this article",
	"this article",
	"educational:",
	"contoh:",
	"penjelasan:",
	"tutorial:",
	"misalkan",
	"misalnya",
	"sebagai ilustrasi",
}

func Detect(content string) Result {
	r := Result{AlgoVersion: AlgoVersion}
	if content == "" {
		r.Severity = SeveritySafe
		r.Allowed = true
		return r
	}

	const maxScan = 64 * 1024
	scanContent := content
	if len(scanContent) > maxScan {
		scanContent = scanContent[:maxScan]
	}

	headLow := strings.ToLower(scanContent)
	if len(headLow) > 200 {
		headLow = headLow[:200]
	}
	for _, p := range educationalPrefixes {
		if strings.Contains(headLow, p) {
			r.EducationalContext = true
			break
		}
	}

	for _, sig := range signatures {
		matches := sig.Re.FindAllString(scanContent, -1)
		if len(matches) == 0 {
			continue
		}
		for i, m := range matches {
			if i >= 3 {
				break
			}
			snippet := m
			if len(snippet) > 80 {
				snippet = snippet[:80] + "…"
			}
			r.Hits = append(r.Hits, Hit{
				Category: sig.Category,
				Pattern:  sig.Name,
				Snippet:  snippet,
				Weight:   sig.Weight,
			})
			r.Score += sig.Weight
		}
	}

	if r.EducationalContext {
		hasHighWeight := false
		for _, h := range r.Hits {
			if h.Weight >= 0.5 {
				hasHighWeight = true
				break
			}
		}
		r.Score *= 0.5
		if hasHighWeight && r.Score < 0.4 {
			r.Score = 0.4
		}
	}

	if r.Score > 1.0 {
		r.Score = 1.0
	}

	switch {
	case r.Score >= 0.7:
		r.Severity = SeveritySuspicious
		r.Allowed = false
		r.Reason = "high prompt injection signal — recommend quarantine"
	case r.Score >= 0.4:
		r.Severity = SeverityReview
		r.Allowed = false
		r.Reason = "moderate prompt injection signal — flag for review"
	default:
		r.Severity = SeveritySafe
		r.Allowed = true
	}

	return r
}

func HasInjection(content string) bool {
	if content == "" {
		return false
	}
	const maxScan = 64 * 1024
	if len(content) > maxScan {
		content = content[:maxScan]
	}
	for _, sig := range signatures {
		if sig.Re.MatchString(content) {
			return true
		}
	}
	return false
}
