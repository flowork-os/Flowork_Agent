package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teetah2402/flowork/internal/commands"
)

// handleSlashCommand — intercept /xxx before sending to model.
// Returns handled=true if the input was consumed locally.
//
// Dispatcher tipis: parse cmd+args, lalu coba setiap kategori handler
// (basic, git, diag, misc, sharedchat, session, mcp, github, dst). Setiap
// handler mengembalikan bool (true = sudah di-handle). Inline cases hanya
// untuk command yang butuh early-exit khusus (exit/quit/pause).
func (m *model) handleSlashCommand(input string, existing []tea.Cmd) (bool, tea.Model, tea.Cmd) {
	raw := strings.TrimPrefix(strings.TrimSpace(input), "/")
	if raw == "" {
		return false, *m, nil
	}
	parts := strings.Fields(raw)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	// Early-exit commands first (need explicit tea.Quit + finalize).
	switch cmd {
	case "exit", "quit", "q":
		m.finalizeSession()
		return true, *m, tea.Quit
	case "pause":
		m.finalizeSession()
		m.appendLocal("PAUSE", "Session state saved. You can resume with: flowork --continue")
		return true, *m, tea.Quit
	}

	// Categorical dispatchers — first match wins.
	if m.handleBasicSlash(cmd, args) {
		m.input.SetValue("")
		return true, *m, tea.Batch(existing...)
	}
	if m.handleGitSlash(cmd, args) {
		m.input.SetValue("")
		return true, *m, tea.Batch(existing...)
	}
	if m.handleDiagSlash(cmd, args) {
		m.input.SetValue("")
		return true, *m, tea.Batch(existing...)
	}
	if m.handleMiscSlash(cmd, args) {
		m.input.SetValue("")
		return true, *m, tea.Batch(existing...)
	}
	if m.handleSharedChatSlash(cmd, args) {
		m.input.SetValue("")
		return true, *m, tea.Batch(existing...)
	}

	// Commands that delegate to existing helper methods in slash_*.go.
	switch cmd {
	// ─── Phase 09: Session Persistence ──────────────────────────────
	case "resume":
		m.handleResumeCommand(args)
	case "tag":
		if len(args) == 0 {
			m.appendLocal("TAG", fmt.Sprintf("Current tags: %s\nUsage: /tag <name>", strings.Join(m.session.Tags, ", ")))
		} else {
			tagName := args[0]
			m.session.Tag(tagName)
			if m.sessionStore != nil {
				_ = m.sessionStore.UpdateTags(m.session.ID, m.session.Tags)
			}
			m.appendLocal("TAG", fmt.Sprintf("Tagged session with %q. Tags: %s", tagName, strings.Join(m.session.Tags, ", ")))
		}
	case "sessions":
		m.handleSessionsListCommand(args)
	case "export":
		m.handleExportCommand(args)
	case "rewind":
		m.handleRewindCommand(args)

	// ─── Phase 10: MCP ─────────────────────────────────────────────
	case "mcp":
		m.handleMCPCommand(args)

	// ─── Phase 12: Integrations ────────────────────────────────────
	case "install-github-app":
		m.handleGitHubInstall()
	case "pr":
		m.handlePRCommand(args)

	// ─── Claude-Code parity commands (Indonesia) ────────────────────────
	case "init":
		m.handleInitCommand()
	case "review":
		m.handleReviewCommand(args)
	case "security-review", "secreview":
		m.handleSecurityReviewCommand()
	case "insights":
		m.handleInsightsCommand()
	case "team-onboarding", "onboarding":
		m.handleTeamOnboardingCommand()
	case "config":
		m.handleConfigCommand(args)
	case "keybindings", "keys":
		m.handleKeybindingsCommand()
	case "release-notes", "changelog":
		m.handleReleaseNotesCommand()
	case "share":
		m.handleShareCommand()
	case "ultraplan":
		m.handleUltraplanCommand()
	case "perf-issue", "perf":
		m.handlePerfIssueCommand()
	case "ctx_viz", "ctxviz":
		m.handleCtxVizCommand()

	// ─── Roadmap gap commands ────────────────────────────────────────
	case "rename":
		m.handleRenameCommand(args)
	case "issue":
		m.handleIssueCommand(args)
	case "summary":
		m.handleSummaryCommand()
	case "autofix-pr":
		m.handleAutofixPRCommand(args)
	case "sandbox-toggle", "sandbox":
		m.handleSandboxToggle()
	case "teleport", "tp":
		m.handleTeleportCommand(args)
	case "tool-usage", "toolusage":
		m.handleToolUsageCommand()
	case "update":
		m.handleUpdateCommand(args)
	case "rollback":
		m.handleRollbackCommand()
	case "sync":
		m.handleSyncCommand(args)

	default:
		// #2 — cek user-defined slash command dari .flowork/commands/*.md
		// sebelum akhirnya bilang unknown. Memungkinkan owner menambah
		// command custom tanpa rebuild.
		if custom := commands.Lookup(cmd); custom != nil {
			expanded := custom.Expand(strings.Join(args, " "))
			m.input.SetValue(expanded)
			m.appendLocal(strings.ToUpper(cmd), fmt.Sprintf("Prompt dari %s disiapkan di input (%s). Enter untuk eksekusi.", filepath.Base(custom.Source), custom.Description))
			return true, *m, tea.Batch(existing...)
		}
		m.appendLocal("UNKNOWN", fmt.Sprintf("Unknown slash command /%s. Try /help", cmd))
	}

	m.input.SetValue("")
	return true, *m, tea.Batch(existing...)
}

// appendLocal — add a locally-generated entry to the transcript (not sent to model).
func (m *model) appendLocal(title, body string) {
	m.entries = append(m.entries, transcriptEntry{
		Role:  "system",
		Title: title,
		Body:  body,
		Meta:  "slash",
	})
}
