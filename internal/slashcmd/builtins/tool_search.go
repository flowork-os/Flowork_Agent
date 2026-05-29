// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 13 phase 1 (Tool discovery). API stable: /tool_search
//   <query> matches name/capability/description substring. Phase 2
//   (fuzzy match, scoring, semantic) → tambah file baru, JANGAN modify.
//
// tool_search.go — Section 13 phase 1: tool discovery via slash command.
//
// Pattern: agent (atau Mr.Dev via Telegram) ngga inget exact tool name —
// `/tool_search net` cari semua tool dengan substring "net" di name,
// capability, atau description. Output sorted by registry order.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/slashcmd"
	"flowork-gui/internal/tools"
)

// toolSearchCmd — /tool_search slash command (Section 13 phase 1).
type toolSearchCmd struct{}

func (toolSearchCmd) Name() string      { return "tool_search" }
func (toolSearchCmd) Aliases() []string { return []string{"ts", "find_tool"} }
func (toolSearchCmd) Description() string {
	return "Search builtin tools by name/capability/description substring. /tool_search <query>"
}

func (toolSearchCmd) Run(_ context.Context, argsRaw string) (slashcmd.Result, error) {
	query := strings.ToLower(strings.TrimSpace(argsRaw))
	if query == "" {
		return slashcmd.Result{}, fmt.Errorf("usage: /tool_search <query>")
	}
	all := tools.ListSummaries()
	matches := []tools.ToolSummary{}
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Name), query) ||
			strings.Contains(strings.ToLower(s.Capability), query) ||
			strings.Contains(strings.ToLower(s.Description), query) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 0 {
		return slashcmd.Result{
			Text:   fmt.Sprintf("_no tools match `%s`_", query),
			Format: "markdown",
		}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Tool search `%s` — %d match:**\n\n", query, len(matches))
	for _, s := range matches {
		cap := s.Capability
		if cap == "" {
			cap = "(none)"
		}
		fmt.Fprintf(&b, "- `%s` (%s)\n", s.Name, cap)
		if s.Description != "" {
			fmt.Fprintf(&b, "  _%s_\n", s.Description)
		}
	}
	return slashcmd.Result{Text: b.String(), Format: "markdown"}, nil
}

// InitToolSearch — register Section 13 /tool_search. Caller (builtins.Init)
// panggil setelah Tier 1.
func InitToolSearch() {
	slashcmd.Register(&toolSearchCmd{})
}
