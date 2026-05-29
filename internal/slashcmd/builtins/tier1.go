// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 15 phase 1 (5 Tier 1 productive slash commands).
//   API stable: /version, /now, /stats, /tools, /interactions. Komutan
//   yang butuh per-agent DB akses via slashcmd.FromStore(ctx). Phase
//   2+ commands (/search, /mistakes-add, /agents) → tambah file baru
//   di package ini, register di tier1.go::InitTier1() OR builtins.go
//   Init(). JANGAN modify existing.
//
// tier1.go — Section 15 phase 1: 5 productive slash commands.
//
// Commands:
//   /version           — daemon version + tools count + slash count
//   /now               — current time (RFC3339 + WIB local)
//   /stats             — karma metrics + interactions/decisions/mistakes count
//   /tools             — list builtin tools dengan capability
//   /interactions      — recent telegram in/out summary (last 10)

package builtins

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"flowork-gui/internal/slashcmd"
	"flowork-gui/internal/tools"
)

// AgentVersion — bumped tiap stable release. Synced dengan main.go const.
const AgentVersion = "0.4.0-embedded-kernel"

// InitTier1 — register Tier 1 commands. Caller (builtins.Init) panggil
// after demo commands registered.
func InitTier1() {
	slashcmd.Register(&versionCmd{})
	slashcmd.Register(&nowCmd{})
	slashcmd.Register(&statsCmd{})
	slashcmd.Register(&toolsCmd{})
	slashcmd.Register(&interactionsCmd{})
}

// =============================================================================
// /version — daemon info + counts
// =============================================================================

type versionCmd struct{}

func (versionCmd) Name() string      { return "version" }
func (versionCmd) Aliases() []string { return []string{"ver", "v"} }
func (versionCmd) Description() string {
	return "Daemon version, tools/slash count, agent ID."
}
func (versionCmd) Run(ctx context.Context, _ string) (slashcmd.Result, error) {
	agent := slashcmd.FromAgent(ctx)
	if agent == "" {
		agent = "(unknown)"
	}
	toolCount := tools.Count()
	slashCount := slashcmd.Count()
	text := fmt.Sprintf(
		"**Flowork Agent %s**\n\n"+
			"- agent_id: `%s`\n"+
			"- tools registered: %d\n"+
			"- slash commands: %d\n"+
			"- tools algo: %s\n"+
			"- slash algo: %s",
		AgentVersion, agent, toolCount, slashCount,
		tools.AlgoVersion, slashcmd.AlgoVersion,
	)
	return slashcmd.Result{Text: text, Format: "markdown"}, nil
}

// =============================================================================
// /now — current time (UTC + WIB)
// =============================================================================

type nowCmd struct{}

func (nowCmd) Name() string        { return "now" }
func (nowCmd) Aliases() []string   { return []string{"time", "date"} }
func (nowCmd) Description() string { return "Server clock: UTC RFC3339 + WIB local." }
func (nowCmd) Run(_ context.Context, _ string) (slashcmd.Result, error) {
	t := time.Now()
	utc := t.UTC()
	// WIB = UTC+7 (Jakarta).
	wib := utc.Add(7 * time.Hour)
	text := fmt.Sprintf(
		"**Server clock**\n\n"+
			"- UTC: `%s`\n"+
			"- WIB: `%s` (UTC+7)\n"+
			"- unix_ms: %d",
		utc.Format(time.RFC3339),
		wib.Format("2006-01-02 15:04:05"),
		t.UnixMilli(),
	)
	return slashcmd.Result{Text: text, Format: "markdown"}, nil
}

// =============================================================================
// /stats — karma + per-table count
// =============================================================================

type statsCmd struct{}

func (statsCmd) Name() string      { return "stats" }
func (statsCmd) Aliases() []string { return []string{"status"} }
func (statsCmd) Description() string {
	return "Karma metrics + interactions/decisions/mistakes/letters/edu counts."
}
func (statsCmd) Run(ctx context.Context, _ string) (slashcmd.Result, error) {
	store, ok := slashcmd.FromStore(ctx)
	if !ok {
		return slashcmd.Result{}, fmt.Errorf("agent store not in context")
	}
	var b strings.Builder
	b.WriteString("**Mr.Flow Stats**\n\n")
	// Karma.
	if karmaList, err := store.ListKarma(); err == nil && len(karmaList) > 0 {
		b.WriteString("**Karma:**\n")
		for _, k := range karmaList {
			fmt.Fprintf(&b, "- `%s` = %.2f (n=%d)\n", k.MetricKey, k.MetricValue, k.MetricCount)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("**Karma:** _no metrics yet_\n\n")
	}
	// Counts.
	b.WriteString("**Counts (non-deleted):**\n")
	if n, err := store.CountInteractions(); err == nil {
		fmt.Fprintf(&b, "- interactions: %d\n", n)
	}
	if n, err := store.CountDecisions(); err == nil {
		fmt.Fprintf(&b, "- decisions: %d\n", n)
	}
	if n, err := store.CountMistakes(""); err == nil {
		fmt.Fprintf(&b, "- mistakes: %d\n", n)
	}
	if n, err := store.CountLetters(false); err == nil {
		fmt.Fprintf(&b, "- death letters: %d\n", n)
	}
	if n, err := store.CountEduErrors(); err == nil {
		fmt.Fprintf(&b, "- edu_errors: %d\n", n)
	}
	if n, err := store.CountToolInvocations(""); err == nil {
		fmt.Fprintf(&b, "- tool_invocations: %d\n", n)
	}
	return slashcmd.Result{Text: b.String(), Format: "markdown"}, nil
}

// =============================================================================
// /tools — list builtin tools dengan capability
// =============================================================================

type toolsCmd struct{}

func (toolsCmd) Name() string        { return "tools" }
func (toolsCmd) Aliases() []string   { return nil }
func (toolsCmd) Description() string { return "List builtin tools dengan capability." }
func (toolsCmd) Run(_ context.Context, _ string) (slashcmd.Result, error) {
	summaries := tools.ListSummaries()
	if len(summaries) == 0 {
		return slashcmd.Result{Text: "_no tools registered_", Format: "markdown"}, nil
	}
	// Group by capability prefix (first segment).
	groups := map[string][]string{}
	for _, s := range summaries {
		cap := s.Capability
		if cap == "" {
			cap = "(none)"
		}
		// First segment as group (e.g. "fs:read:/x" → "fs")
		key := strings.SplitN(cap, ":", 2)[0]
		groups[key] = append(groups[key], s.Name)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	fmt.Fprintf(&b, "**Builtin tools (%d):**\n\n", len(summaries))
	for _, k := range keys {
		names := groups[k]
		sort.Strings(names)
		fmt.Fprintf(&b, "- **%s**: `%s`\n", k, strings.Join(names, "`, `"))
	}
	return slashcmd.Result{Text: b.String(), Format: "markdown"}, nil
}

// =============================================================================
// /interactions — recent Telegram in/out summary
// =============================================================================

type interactionsCmd struct{}

func (interactionsCmd) Name() string      { return "interactions" }
func (interactionsCmd) Aliases() []string { return []string{"chat", "history"} }
func (interactionsCmd) Description() string {
	return "Last 10 Telegram interactions (in/out + actor + preview)."
}
func (interactionsCmd) Run(ctx context.Context, _ string) (slashcmd.Result, error) {
	store, ok := slashcmd.FromStore(ctx)
	if !ok {
		return slashcmd.Result{}, fmt.Errorf("agent store not in context")
	}
	items, err := store.ListInteractions("telegram", "", 10)
	if err != nil {
		return slashcmd.Result{}, fmt.Errorf("list interactions: %w", err)
	}
	if len(items) == 0 {
		return slashcmd.Result{Text: "_no interactions yet_", Format: "markdown"}, nil
	}
	var b strings.Builder
	b.WriteString("**Last 10 interactions:**\n\n")
	for _, it := range items {
		// Truncate preview to 60 char.
		preview := strings.ReplaceAll(it.Content, "\n", " ")
		if len(preview) > 60 {
			preview = preview[:60] + "…"
		}
		fmt.Fprintf(&b, "- `%s` `%s` (%s): %s\n",
			it.OccurredAt, it.Direction, it.Actor, preview)
	}
	return slashcmd.Result{Text: b.String(), Format: "markdown"}, nil
}
