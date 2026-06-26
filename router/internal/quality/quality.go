// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package quality

import (
	"strings"
	"unicode"
)

const AlgoVersion = "v1"

type Result struct {
	AlgoVersion string  `json:"algo_version"`
	Allowed     bool    `json:"allowed"`
	Reason      string  `json:"reason,omitempty"`
	Score       float64 `json:"score"`

	LengthScore     float64 `json:"length_score"`
	RepetitionScore float64 `json:"repetition_score"`
	WhitespaceScore float64 `json:"whitespace_score"`
	DiversityScore  float64 `json:"diversity_score"`
}

const (
	minLengthBytes    = 20
	maxLengthBytes    = 256 * 1024
	maxRepetitionPct  = 0.30
	maxWhitespacePct  = 0.80
	minDiversityChars = 5
	maxDiversityCount = 20
	overallThreshold  = 0.5
)

func Check(content string) Result {
	r := Result{AlgoVersion: AlgoVersion}

	if content == "" {
		r.Reason = "content empty"
		return r
	}

	n := len(content)
	switch {
	case n < minLengthBytes:
		r.Reason = "content too short"
		r.LengthScore = 0
	case n > maxLengthBytes:
		r.Reason = "content too long"
		r.LengthScore = 0
	default:

		switch {
		case n < 100:
			r.LengthScore = float64(n) / 100.0
		case n <= 10000:
			r.LengthScore = 1.0
		default:

			r.LengthScore = 1.0 - 0.5*float64(n-10000)/float64(maxLengthBytes-10000)
		}
	}
	if r.Reason != "" {
		return r
	}

	wsCount := 0
	runeCount := 0
	unique := make(map[rune]struct{}, maxDiversityCount+1)
	diversityCapped := false
	for _, c := range content {
		runeCount++
		if unicode.IsSpace(c) {
			wsCount++
			continue
		}
		if !diversityCapped {
			unique[c] = struct{}{}
			if len(unique) >= maxDiversityCount {
				diversityCapped = true
			}
		}
	}
	if runeCount == 0 {
		runeCount = 1
	}
	wsRatio := float64(wsCount) / float64(runeCount)
	if wsRatio > maxWhitespacePct {
		r.Reason = "content mostly whitespace"
		return r
	}
	r.WhitespaceScore = 1.0 - wsRatio

	if len(unique) < minDiversityChars {
		r.Reason = "content low diversity (likely garbage/spam)"
		return r
	}

	div := float64(len(unique))
	if div > maxDiversityCount {
		div = maxDiversityCount
	}
	r.DiversityScore = div / float64(maxDiversityCount)

	repPct := detectRepetition(content)
	if repPct > maxRepetitionPct {
		r.Reason = "content high repetition (likely spam)"
		return r
	}
	r.RepetitionScore = 1.0 - repPct

	r.Score = (r.LengthScore + r.WhitespaceScore + r.DiversityScore + r.RepetitionScore) / 4.0
	r.Allowed = r.Score >= overallThreshold
	if !r.Allowed {
		r.Reason = "composite quality score below threshold"
	}
	return r
}

func detectRepetition(content string) float64 {
	if len(content) < 6 {
		return 0
	}

	const maxScan = 16 * 1024
	if len(content) > maxScan {
		content = content[:maxScan]
	}

	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, content)
	if len(cleaned) < 6 {
		return 0
	}

	counts := map[string]int{}
	total := 0
	for i := 0; i+3 <= len(cleaned); i++ {
		gram := cleaned[i : i+3]
		counts[gram]++
		total++
	}
	if total == 0 {
		return 0
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	return float64(maxCount) / float64(total)
}
