// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 14 phase 1 (Dispatcher). API stable: Dispatch(ctx,
//   text) → (Result, cmdName, error). Parse: strip leading "/", split
//   first token as name, rest as argsRaw. Caseflexible (lowercase
//   lookup). Phase 2 (auto-suggest fallback to skill catalog, fuzzy
//   match) → tambah file baru.
//
// dispatcher.go — slash parse + dispatch.
//
// USAGE:
//   result, err := slashcmd.Dispatch(ctx, "/help")
//   result, err := slashcmd.Dispatch(ctx, "/search foo bar")
//
// Parse: skip leading "/", split first token = name, rest = argsRaw.
// Lookup via registry. Run.

package slashcmd

import (
	"context"
	"fmt"
	"strings"
)

// Dispatch — main entry point. Caller pass full text including leading "/".
// Return Result + error.
//
// Empty or non-slash input → error "not a slash command".
func Dispatch(ctx context.Context, text string) (Result, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, "", fmt.Errorf("empty input")
	}
	if !strings.HasPrefix(text, "/") {
		return Result{}, "", fmt.Errorf("not a slash command (missing leading '/')")
	}
	// Strip leading "/" + split first token.
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
