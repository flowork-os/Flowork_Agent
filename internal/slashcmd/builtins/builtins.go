// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 14 phase 1 (3 demo commands). API stable: Init()
//   registers help/echo/ping. Phase 2 (Tier 1 commands /search /stats
//   /list /agents dst.) → tambah file baru, register di Init().
//
// Package builtins — Section 14 phase 1: 3 demo slash commands.
//
// Commands:
//   /help               — list all registered commands + descriptions
//   /echo <text>        — echo back input
//   /ping               — health check (returns "pong")
//
// Real tier 1 commands (/search /stats /list /agents dst.) di Section 15.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/slashcmd"
)

// Init — explicit bootstrap. Caller (main.go) panggil exactly once.
func Init() {
	slashcmd.Register(&helpCmd{})
	slashcmd.Register(&echoCmd{})
	slashcmd.Register(&pingCmd{})
	// Section 15 phase 1: 5 Tier 1 productive commands.
	InitTier1()
	// Section 13 phase 1: /tool_search discovery command.
	InitToolSearch()
}

// =============================================================================
// /help
// =============================================================================

type helpCmd struct{}

func (helpCmd) Name() string        { return "help" }
func (helpCmd) Aliases() []string   { return []string{"h", "?"} }
func (helpCmd) Description() string { return "List all registered slash commands." }
func (helpCmd) Run(_ context.Context, _ string) (slashcmd.Result, error) {
	var b strings.Builder
	b.WriteString("**Available slash commands:**\n\n")
	for _, s := range slashcmd.ListSummaries() {
		fmt.Fprintf(&b, "- `/%s`", s.Name)
		if len(s.Aliases) > 0 {
			fmt.Fprintf(&b, " (aliases: %s)", strings.Join(s.Aliases, ", "))
		}
		fmt.Fprintf(&b, " — %s\n", s.Description)
	}
	return slashcmd.Result{
		Text:   b.String(),
		Format: "markdown",
	}, nil
}

// =============================================================================
// /echo
// =============================================================================

type echoCmd struct{}

func (echoCmd) Name() string        { return "echo" }
func (echoCmd) Aliases() []string   { return nil }
func (echoCmd) Description() string { return "Echo back the input text. /echo hello → hello" }
func (echoCmd) Run(_ context.Context, argsRaw string) (slashcmd.Result, error) {
	if argsRaw == "" {
		return slashcmd.Result{}, fmt.Errorf("usage: /echo <text>")
	}
	return slashcmd.Result{Text: argsRaw}, nil
}

// =============================================================================
// /ping
// =============================================================================

type pingCmd struct{}

func (pingCmd) Name() string        { return "ping" }
func (pingCmd) Aliases() []string   { return []string{"pong"} }
func (pingCmd) Description() string { return "Health check. Always returns 'pong'." }
func (pingCmd) Run(_ context.Context, _ string) (slashcmd.Result, error) {
	return slashcmd.Result{Text: "pong"}, nil
}
