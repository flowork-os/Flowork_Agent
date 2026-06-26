// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package bypass

import (
	"strings"
)

type Message struct {
	Role    string
	Content string
}

type Decision struct {
	Bypass bool

	NamingTitle string

	Reason string
}

func Detect(messages []Message, userAgent string, skipPatterns []string, ccFilterNaming bool) Decision {
	if !strings.Contains(userAgent, "claude-cli") || len(messages) == 0 {
		return Decision{}
	}

	last := messages[len(messages)-1]
	if last.Role == "assistant" && strings.TrimSpace(last.Content) == "{" {
		return Decision{Bypass: true, Reason: "title"}
	}

	first := messages[0]
	if strings.TrimSpace(first.Content) == "Warmup" {
		return Decision{Bypass: true, Reason: "warmup"}
	}

	if len(messages) == 1 && messages[0].Role == "user" && strings.TrimSpace(messages[0].Content) == "count" {
		return Decision{Bypass: true, Reason: "count"}
	}

	if len(skipPatterns) > 0 {
		var sb strings.Builder
		for _, m := range messages {
			if m.Role == "user" {
				sb.WriteString(m.Content)
				sb.WriteByte(' ')
			}
		}
		joined := sb.String()
		for _, p := range skipPatterns {
			if p = strings.TrimSpace(p); p != "" && strings.Contains(joined, p) {
				return Decision{Bypass: true, Reason: "skip-pattern"}
			}
		}
	}

	if ccFilterNaming {
		var systemText strings.Builder
		var firstUser string
		for _, m := range messages {
			if m.Role == "system" {
				systemText.WriteString(m.Content)
				systemText.WriteByte(' ')
			}
			if firstUser == "" && m.Role == "user" {
				firstUser = m.Content
			}
		}
		if strings.Contains(systemText.String(), "isNewTopic") {
			return Decision{
				Bypass:      true,
				Reason:      "naming",
				NamingTitle: firstThreeWords(firstUser),
			}
		}
	}

	return Decision{}
}

const DefaultStubText = "CLI Command Execution: Clear Terminal"

func StubText(d Decision) string {
	if d.NamingTitle != "" {

		return `{"isNewTopic":true,"title":` + jsonQuote(d.NamingTitle) + `}`
	}
	return DefaultStubText
}

func firstThreeWords(s string) string {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, " ")
}

func jsonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
