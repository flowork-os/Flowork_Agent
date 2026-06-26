// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/slashcmd"
	"flowork-gui/internal/tools"
)

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

func InitToolSearch() {
	slashcmd.Register(&toolSearchCmd{})
}
