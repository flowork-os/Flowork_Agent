// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 14 phase 1 (interface). API stable: SlashCommand
//   interface (Name/Aliases/Description/Run), Result struct (Text +
//   Format markdown/plain/json). Phase 2 (interactive multi-step, stream
//   output) → tambah optional interface di file lain, JANGAN modify ini.
//
// Package slashcmd — Section 14 phase 1: slash command foundation.
//
// PURPOSE:
//   Skeleton untuk slash command dispatch. Phase 1 = interface +
//   registry + dispatcher + 3 demo commands. Real commands tier 1
//   (/help /list /stats /search dst) di Section 15.
//
// DESIGN:
//   Mirror tools/types.go pattern — interface SlashCommand dengan
//   Name/Aliases/Description/Run. Plug-and-play: register di init()
//   atau bootstrap() call.

package slashcmd

import "context"

// AlgoVersion — slash system version.
const AlgoVersion = "v1"

// Result — slash command output. Text untuk Telegram/CLI render.
type Result struct {
	Text   string `json:"text"`              // human-readable response
	Format string `json:"format,omitempty"`  // 'plain' | 'markdown' | 'json'
}

// SlashCommand — interface yang setiap command harus penuhi.
type SlashCommand interface {
	// Name — canonical command (lowercase, no leading slash).
	// E.g. "help", "stats", "search".
	Name() string
	// Aliases — alternative names (lowercase). Lookup by alias jatuh ke
	// canonical via registry.
	Aliases() []string
	// Description — 1-line summary buat /help list.
	Description() string
	// Run — execute. argsRaw = raw text after command name (mis. "/search
	// foo bar" → argsRaw="foo bar"). Caller ctx untuk cancel.
	Run(ctx context.Context, argsRaw string) (Result, error)
}
