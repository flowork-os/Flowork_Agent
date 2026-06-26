// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package ingest

import (
	"strings"
)

var signalWords = []string{
	"bug", "fix", "incident", "rootcause", "root cause", "regression",
	"security", "vulnerability", "cve", "exploit", "patch", "mitigation",
	"deprecated", "breaking change", "migration", "rollback",
	"decision", "rfc", "design doc", "architecture",
	"policy", "constitution", "governance", "amendment",
	"warning", "caution", "important", "critical",
	"todo", "fixme", "hack", "xxx",
}

var sourceTypeBoost = map[string]float64{
	"manual":      2.0,
	"doc":         1.5,
	"federation":  1.0,
	"chat":        0.5,
	"compounding": 0.5,
	"":            1.0,
}

func Score(content, sourceType string) float64 {
	if content == "" {
		return 0.5
	}
	score := 3.0

	if b, ok := sourceTypeBoost[strings.ToLower(sourceType)]; ok {
		score += b
	} else {
		score += sourceTypeBoost[""]
	}

	lower := strings.ToLower(content)
	hits := 0
	for _, w := range signalWords {
		if strings.Contains(lower, w) {
			hits++
			if hits >= 4 {
				break
			}
		}
	}
	score += float64(hits) * 0.5

	switch n := len(content); {
	case n < 50:
		score -= 1.5
	case n < 200:

	case n < 1500:
		score += 1.0
	default:
		score += 0.5
	}

	if score < 0.5 {
		score = 0.5
	}
	if score > 10.0 {
		score = 10.0
	}
	return score
}
