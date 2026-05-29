package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/compact"
	"github.com/teetah2402/flowork/internal/core"
	"github.com/teetah2402/flowork/internal/fsutil"
	"github.com/teetah2402/flowork/internal/outputstyle"
	"github.com/teetah2402/flowork/internal/pricing"
	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/tools"
)

// handleBasicSlash menangani slash commands kategori dasar:
// help/clear/unlock/tools/skills/provider/model/permissions/status/memory/save.
// Returns true kalau cmd dikenali (handled), false kalau bukan kategori ini.
func (m *model) handleBasicSlash(cmd string, args []string) bool {
	switch cmd {
	case "help", "h", "?":
		m.appendLocal("HELP", slashHelpText())
	case "clear", "cls":
		m.entries = nil
		m.appendLocal("CLEARED", "Conversation history cleared.")
	case "unlock":
		// rc23 Bug-B-C2: emergency escape hatch for the composer
		// freeze that defeats all our runtime guards. If bubbletea
		// still thinks a turn is in flight (m.busy stuck true) and
		// the user can't press Enter to send, they can still type
		// /unlock + Enter — the slash interceptor fires before the
		// busy check in startTurn, so this is the one command that
		// remains reachable. Forces a clean state regardless of how
		// we got here.
		if m.turnCancel != nil {
			m.turnCancel()
			m.turnCancel = nil
		}
		m.turnUpdates = nil
		m.busy = false
		m.queuedPrompt = ""
		m.streamingEntryIdx = -1
		m.activePane = paneComposer
		_ = m.input.Focus()
		m.resetEscArm()
		m.status = "Ready for the next prompt"
		m.lastAction = "Manually unlocked via /unlock."
		m.lastErr = nil
		m.appendLocal("UNLOCKED", "Turn state forcibly reset. Composer ready; any in-flight agent goroutine has been ctx-canceled. Type your next prompt.")
	case "tools":
		m.appendLocal("TOOLS", listToolsText())
	case "skills":
		m.appendLocal("SKILLS", listSkillsText())
	case "provider":
		m.appendLocal("PROVIDER", fmt.Sprintf("Current: %s / %s", m.provider, m.modelName))
	case "model":
		if len(args) > 0 {
			newModel := args[0]
			// Runtime model switch — hanya berlaku kalau agent masih local
			// *core.Agent (bukan remote thin-client). Remote mode perlu
			// restart flowork-chat untuk ganti model karena agent hidup di
			// proses lain.
			if localAgent, ok := m.agent.(*core.Agent); ok {
				old := localAgent.Model()
				localAgent.SetModel(newModel)
				m.modelName = newModel
				m.appendLocal("MODEL", fmt.Sprintf("Model: %s → %s (berlaku mulai turn berikutnya)", old, newModel))
			} else {
				m.appendLocal("MODEL", "Remote mode — restart flowork-chat untuk ganti model. Current: "+m.modelName)
			}
		} else {
			m.appendLocal("MODEL", fmt.Sprintf("Current model: %s", m.modelName))
		}
	case "permissions", "perm":
		mode := tools.CurrentPermissionMode()
		if len(args) > 0 {
			switch args[0] {
			case "yolo", "bypass":
				tools.SetPermissionMode(tools.PermissionBypass)
				_ = os.Setenv("FLOW_BYPASS_PROMPT", "1")
			case "plan":
				tools.SetPermissionMode(tools.PermissionPlan)
			case "acceptedits", "accept-edits":
				tools.SetPermissionMode(tools.PermissionAcceptEdits)
			case "default":
				tools.SetPermissionMode(tools.PermissionDefault)
			default:
				m.appendLocal("PERM", fmt.Sprintf("Unknown mode %q. Use: yolo | plan | accept-edits | default", args[0]))
				return true
			}
			m.appendLocal("PERM", fmt.Sprintf("Mode: %s → %s", mode, tools.CurrentPermissionMode()))
		} else {
			m.appendLocal("PERM", fmt.Sprintf("Current: %s (use /perm yolo|plan|accept-edits|default to change)", mode))
		}
	case "status":
		m.appendLocal("STATUS", fmt.Sprintf(
			"Provider: %s\nModel: %s\nWorkspace: %s\nTurns: %d\nSteps this turn: %d\nSession tokens: in=%d out=%d\nPermission: %s\nBusy: %v\nSession ID: %s",
			m.provider, m.modelName, m.workspace, m.turnCount, m.stepCount,
			m.sessionUsage.InputTokens, m.sessionUsage.OutputTokens,
			tools.CurrentPermissionMode(), m.busy, m.session.ID,
		))
	case "memory", "mem":
		m.appendLocal("MEMORY", showMemoryText())
	case "save":
		path := filepath.Join(".flowork-session-" + time.Now().Format("20060102-150405") + ".md")
		if err := saveTranscript(m.entries, path); err != nil {
			m.appendLocal("SAVE", "Error: "+err.Error())
		} else {
			m.appendLocal("SAVE", "Saved to "+path)
		}
	case "exec", "!":
		if len(args) == 0 {
			m.appendLocal("EXEC", "Usage: /exec <shell command>")
			return true
		}
		m.appendLocal("EXEC", fmt.Sprintf("Note: direct shell exec from slash not yet wired. Use bash tool via AI instead: 'run: %s'", strings.Join(args, " ")))
	case "version":
		m.appendLocal("VERSION", fmt.Sprintf("FLOWORK Go v0.3.0\nBuild: %s/%s\nProvider: %s\nModel: %s", runtime.GOOS, runtime.GOARCH, m.provider, m.modelName))
	case "session":
		m.appendLocal("SESSION", fmt.Sprintf("ID:       %s\nProvider: %s\nModel:    %s\nTurns:    %d\nStarted:  %s",
			m.session.ID, m.provider, m.modelName, m.turnCount,
			m.session.StartedAt.Format("2006-01-02 15:04:05")))
	case "output-style":
		if len(args) == 0 {
			m.appendLocal("OUTPUT-STYLE", fmt.Sprintf("Current: %s\nUsage: /output-style <default|concise|explanatory>", outputstyle.Current()))
		} else {
			newMode := outputstyle.Set(args[0])
			// Inject instruksi style ke session sebagai system note agar
			// model langsung menyesuaikan mulai turn berikutnya — tanpa
			// harus refactor session.SystemPrompt assembly.
			if appendix := outputstyle.SystemAppendix(); appendix != "" {
				m.session.Add(provider.Message{
					Role:    provider.RoleSystem,
					Content: "[owner changed output style]" + appendix,
				})
			}
			m.appendLocal("OUTPUT-STYLE", fmt.Sprintf("Mode: %s (instruksi sudah diinject ke session)", newMode))
		}
	case "stats", "usage", "cost":
		// #14 — pakai tabel harga internal/pricing yang sudah include
		// cache_read + cache_create pricing per Aksara convention.
		cost := pricing.Estimate(m.modelName, m.sessionUsage)
		m.appendLocal(strings.ToUpper(cmd), fmt.Sprintf(
			"Session tokens:   %s in / %s out\nCache:            %d read / %d create\nTool calls:       %d\nTurns:            %d\nEst. cost:        %s\nProvider:         %s / %s\n\n(Estimate berdasarkan harga publik per April 2026 — aktual bisa beda)",
			formatTokenCount(m.sessionUsage.InputTokens),
			formatTokenCount(m.sessionUsage.OutputTokens),
			m.sessionUsage.CacheReadInputTokens, m.sessionUsage.CacheCreationInputTokens,
			m.sessionToolCalls, m.turnCount, pricing.Format(cost),
			m.provider, m.modelName,
		))
	case "effort":
		total := m.sessionUsage.InputTokens + m.sessionUsage.OutputTokens
		limit := compact.ModelContextLimit(m.modelName)
		remaining := limit - total
		if remaining < 0 {
			remaining = 0
		}
		m.appendLocal("EFFORT", fmt.Sprintf(
			"Tokens used:      %s / ~%s (%.1f%%)\nTokens remaining: ~%s\nEst. turns left:  ~%d (at current avg %d tok/turn)",
			formatTokenCount(total), formatTokenCount(limit),
			float64(total)/float64(limit)*100,
			formatTokenCount(remaining),
			safeDiv(remaining, safeDiv(total, max(1, m.turnCount))),
			safeDiv(total, max(1, m.turnCount)),
		))
	default:
		return false
	}
	return true
}

func slashHelpText() string {
	return `Available slash commands (handled locally — not sent to AI):

  /help                   Show this help
  /clear                  Clear conversation history
  /exit, /quit, /q        Exit FLOWORK
  /tools                  List all registered tools
  /skills                 List available skills
  /provider               Show current provider + model
  /model [name]           Show/note model (switch needs restart)
  /permissions [mode]     Show/set: yolo | plan | accept-edits | default
  /status                 Session status (tokens, turns, permission)
  /memory                 Show saved memory notes
  /save                   Save transcript to .flowork-session-*.md

  Session:
  /resume                 Show recent sessions to resume
  /resume <id>            Resume a specific session
  /resume --latest        Resume most recent session
  /tag <name>             Tag current session
  /sessions [--tag name]  List all sessions (optionally filtered)
  /export [--format md|json] [--for-share]  Export session
  /rewind <file>          Restore file from snapshot
  /rewind --session       Rollback all file changes
  /pause                  Save and exit (can resume later)

  MCP:
  /mcp list               Show configured + available MCP servers
  /mcp install <name>     Install official MCP server

  Agents:
  /agents                 List 5 built-in agent types

  Integrations:
  /install-github-app     Setup GitHub authentication
  /pr [title]             Create PR from current changes

  Thinking:
  /fast                   Disable thinking (fast responses)
  /think [budget]         Enable thinking (default 4000 tokens)
  /thinking               Show current thinking mode

  Shared Chat (satu jalur dengan flowork-chat web):
  /chat <pesan>           Kirim pesan ke shared chat (channel aktif)
  /channel [nama]         Lihat atau ganti channel posting
  /private [label]        Mulai sesi privat (channel unik)
  /clearchat              Clear channel aktif (tulis sentinel)

Anything else is sent to the AI as a prompt.`
}

func listToolsText() string {
	// We don't have access to Registry here — hardcode list.
	// Update this when tools are added.
	list := []string{
		"askuserquestion", "bash", "edit", "glob", "grep", "list",
		"mcp_call", "mcp_list_resources", "mcp_read_resource",
		"multiedit", "notebookedit", "read", "skill", "sleep",
		"task", "task_parallel", "todo", "webfetch", "websearch", "write",
	}
	sort.Strings(list)
	return fmt.Sprintf("%d tools registered:\n  %s", len(list), strings.Join(list, ", "))
}

func listSkillsText() string {
	return `5 built-in skills (invoke via 'skill' tool):

  remember   Save user preference to ~/.flowork/memory/
  debug      Systematic bug investigation flow
  verify     Double-check claims before committing
  simplify   Review code for unnecessary complexity
  batch      Run independent tasks in parallel

Usage: AI will call skill tool with {"skill":"debug","args":"..."}.`
}

func showMemoryText() string {
	home, _ := os.UserHomeDir()
	memPath := filepath.Join(home, ".flowork", "memory", "notes.md")
	data, err := fsutil.SafeReadFile(memPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "(no memory yet — AI will save notes to " + memPath + ")"
		}
		return "Error reading memory: " + err.Error()
	}
	s := string(data)
	if len(s) > 4000 {
		s = s[:4000] + "\n... (truncated)"
	}
	return s
}
