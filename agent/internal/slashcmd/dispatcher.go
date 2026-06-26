// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package slashcmd

import (
	"context"
	"fmt"
	"strings"
)

func Dispatch(ctx context.Context, text string) (Result, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, "", fmt.Errorf("empty input")
	}
	if !strings.HasPrefix(text, "/") {
		return Result{}, "", fmt.Errorf("not a slash command (missing leading '/')")
	}

	rest := strings.TrimPrefix(text, "/")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return Result{}, "", fmt.Errorf("empty command after '/'")
	}

	var name, argsRaw string
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		name = rest[:idx]
		argsRaw = strings.TrimSpace(rest[idx+1:])
	} else {
		name = rest
	}
	name = strings.ToLower(name)

	cmd, ok := Lookup(name)
	if !ok {
		return Result{}, name, fmt.Errorf("command not found: /%s", name)
	}

	res, err := cmd.Run(ctx, argsRaw)
	return res, cmd.Name(), err
}
