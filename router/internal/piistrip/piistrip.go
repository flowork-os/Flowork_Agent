// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package piistrip

import (
	"regexp"
	"strings"
)

const AlgoVersion = "v1"

type PIIType string

const (
	TypeEmail      PIIType = "email"
	TypePhoneID    PIIType = "phone_id"
	TypePhoneIntl  PIIType = "phone_intl"
	TypeCreditCard PIIType = "credit_card"
	TypeNIK        PIIType = "nik_id"
	TypeIP         PIIType = "ip"
	TypeURL        PIIType = "url"
)

type patternDef struct {
	Type    PIIType
	Pattern *regexp.Regexp
}

var patterns []patternDef

func init() {

	patterns = []patternDef{

		{TypeURL, regexp.MustCompile(`\bhttps?://[^\s<>"\)]+`)},

		{TypeEmail, regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)},

		{TypePhoneID, regexp.MustCompile(`(?:\+62|62|0)8\d{8,11}\b`)},

		{TypePhoneIntl, regexp.MustCompile(`\+\d{7,15}\b`)},

		{TypeCreditCard, regexp.MustCompile(`\b\d{4}[ -]\d{4}[ -]\d{4}[ -]\d{4}\b`)},

		{TypeNIK, regexp.MustCompile(`\b\d{16}\b`)},

		{TypeIP, regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)},
	}
}

type Result struct {
	AlgoVersion string               `json:"algo_version"`
	Cleaned     string               `json:"cleaned"`
	Counts      map[PIIType]int      `json:"counts"`
	Found       map[PIIType][]string `json:"found,omitempty"`
	Total       int                  `json:"total"`
}

func Strip(content string) Result {
	r := Result{
		AlgoVersion: AlgoVersion,
		Cleaned:     content,
		Counts:      map[PIIType]int{},
		Found:       map[PIIType][]string{},
	}
	if content == "" {
		return r
	}

	for _, p := range patterns {
		token := "[REDACTED:" + string(p.Type) + "]"

		if p.Type == TypeURL {
			count := 0
			r.Cleaned = p.Pattern.ReplaceAllStringFunc(r.Cleaned, func(m string) string {
				trimmed := trimTrailingPunct(m)
				count++
				if len(r.Found[p.Type]) < 3 {
					r.Found[p.Type] = append(r.Found[p.Type], trimmed)
				}
				if trimmed != m {

					return token + m[len(trimmed):]
				}
				return token
			})
			if count > 0 {
				r.Counts[p.Type] = count
				r.Total += count
			}
			continue
		}
		matches := p.Pattern.FindAllString(r.Cleaned, -1)
		if len(matches) == 0 {
			continue
		}
		r.Counts[p.Type] = len(matches)
		r.Total += len(matches)

		for i, m := range matches {
			if i >= 3 {
				break
			}
			r.Found[p.Type] = append(r.Found[p.Type], m)
		}
		r.Cleaned = p.Pattern.ReplaceAllString(r.Cleaned, token)
	}

	return r
}

func trimTrailingPunct(s string) string {
	return strings.TrimRight(s, ".,;:!?")
}

func StripQuiet(content string) (cleaned string, counts map[PIIType]int, total int) {
	cleaned = content
	counts = map[PIIType]int{}
	if content == "" {
		return
	}
	for _, p := range patterns {
		token := "[REDACTED:" + string(p.Type) + "]"
		if p.Type == TypeURL {
			count := 0
			cleaned = p.Pattern.ReplaceAllStringFunc(cleaned, func(m string) string {
				trimmed := trimTrailingPunct(m)
				count++
				if trimmed != m {
					return token + m[len(trimmed):]
				}
				return token
			})
			if count > 0 {
				counts[p.Type] = count
				total += count
			}
			continue
		}
		matches := p.Pattern.FindAllString(cleaned, -1)
		if len(matches) == 0 {
			continue
		}
		counts[p.Type] = len(matches)
		total += len(matches)
		cleaned = p.Pattern.ReplaceAllString(cleaned, token)
	}
	return
}

func HasPII(content string) bool {
	if content == "" {
		return false
	}
	for _, p := range patterns {
		if p.Pattern.MatchString(content) {
			return true
		}
	}
	return false
}
