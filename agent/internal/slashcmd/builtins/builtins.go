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
)

func Init() {
	slashcmd.Register(&helpCmd{})
	slashcmd.Register(&echoCmd{})
	slashcmd.Register(&pingCmd{})

	InitTier1()

	InitToolSearch()
}

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

type pingCmd struct{}

func (pingCmd) Name() string        { return "ping" }
func (pingCmd) Aliases() []string   { return []string{"pong"} }
func (pingCmd) Description() string { return "Health check. Always returns 'pong'." }
func (pingCmd) Run(_ context.Context, _ string) (slashcmd.Result, error) {
	return slashcmd.Result{Text: "pong"}, nil
}
