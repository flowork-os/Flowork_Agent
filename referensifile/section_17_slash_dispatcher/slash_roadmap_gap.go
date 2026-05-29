package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/analytics"
	"github.com/teetah2402/flowork/internal/tools"
)

// ─── /tool-usage — per-tool usage breakdown from analytics ──────────────
func (m *model) handleToolUsageCommand() {
	tracker := analytics.Get()
	m.appendLocal("TOOL-USAGE", tracker.FormatToolUseSummary())
}

// ─── /rename — rename current session ────────────────────────────────────
func (m *model) handleRenameCommand(args []string) {
	if len(args) == 0 {
		m.appendLocal("RENAME", fmt.Sprintf("Current session: %s\nUsage: /rename <new-name>", m.session.ID))
		return
	}
	newName := strings.Join(args, " ")
	if m.sessionStore != nil {
		if err := m.sessionStore.UpdateTitle(m.session.ID, newName); err != nil {
			m.appendLocal("RENAME", "Gagal rename: "+err.Error())
			return
		}
	}
	m.session.Title = newName
	m.appendLocal("RENAME", fmt.Sprintf("✅ Session di-rename ke %q", newName))
}

// ─── /issue — create GitHub issue dari conversation ──────────────────────
func (m *model) handleIssueCommand(args []string) {
	if len(args) == 0 {
		prompt := "Buat GitHub issue dari konteks percakapan ini. Analisis masalah yang dibahas, lalu jalankan: gh issue create --title \"<judul>\" --body \"<deskripsi detail>\". Pastikan deskripsi mencakup: steps to reproduce, expected behavior, actual behavior."
		m.input.SetValue(prompt)
		m.appendLocal("ISSUE", "Prompt issue disiapkan dari konteks percakapan. Enter untuk eksekusi.")
		return
	}
	// Direct title provided
	title := strings.Join(args, " ")
	prompt := fmt.Sprintf("Buat GitHub issue dengan judul %q. Analisis konteks percakapan ini untuk isi deskripsi, lalu jalankan: gh issue create --title %q --body \"<deskripsi dari konteks>\".", title, title)
	m.input.SetValue(prompt)
	m.appendLocal("ISSUE", fmt.Sprintf("Prompt issue %q disiapkan. Enter untuk eksekusi.", title))
}

// ─── /summary — session summary extraction ───────────────────────────────
func (m *model) handleSummaryCommand() {
	prompt := "Rangkum percakapan ini sejauh ini. Buat ringkasan terstruktur: (1) Tujuan utama, (2) Apa yang sudah dikerjakan, (3) Keputusan penting yang diambil, (4) Issue/blocker yang belum selesai, (5) Next steps. Format markdown, ringkas."
	m.input.SetValue(prompt)
	m.appendLocal("SUMMARY", "Prompt summary disiapkan. Enter untuk eksekusi.")
}

// ─── /autofix-pr — auto-fix PR review comments ──────────────────────────
func (m *model) handleAutofixPRCommand(args []string) {
	prNum := ""
	if len(args) > 0 {
		prNum = args[0]
	}
	var prompt string
	if prNum != "" {
		prompt = fmt.Sprintf("Ambil review comments dari PR #%s (pakai `gh pr view %s --comments` dan `gh api repos/{owner}/{repo}/pulls/%s/reviews`). Untuk tiap comment yang actionable: baca file yang dimaksud, implementasikan fix, lalu commit. Abaikan comment yang bersifat diskusi/non-actionable.", prNum, prNum, prNum)
	} else {
		prompt = "Ambil review comments dari PR aktif di branch ini (pakai `gh pr view --comments`). Untuk tiap comment yang actionable: baca file yang dimaksud, implementasikan fix, lalu commit. Abaikan comment yang bersifat diskusi/non-actionable."
	}
	m.input.SetValue(prompt)
	m.appendLocal("AUTOFIX-PR", "Prompt autofix PR disiapkan. Enter untuk eksekusi.")
}

// ─── /teleport — switch workspace root to another directory ─────────────
func (m *model) handleTeleportCommand(args []string) {
	if len(args) == 0 {
		m.appendLocal("TELEPORT", fmt.Sprintf("Usage: /teleport <path>\nCurrent workspace: %s", m.workspace))
		return
	}
	target := strings.TrimSpace(strings.Join(args, " "))
	if !filepath.IsAbs(target) {
		target = filepath.Join(m.workspace, target)
	}
	info, err := os.Stat(target)
	if err != nil {
		m.appendLocal("TELEPORT", fmt.Sprintf("✗ %v", err))
		return
	}
	if !info.IsDir() {
		m.appendLocal("TELEPORT", fmt.Sprintf("✗ not a directory: %s", target))
		return
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		m.appendLocal("TELEPORT", fmt.Sprintf("✗ %v", err))
		return
	}
	old := m.workspace
	m.workspace = abs
	if err := os.Chdir(abs); err != nil {
		m.appendLocal("TELEPORT", fmt.Sprintf("⚠ workspace tracker pindah ke %s tapi os.Chdir gagal: %v\n(tools tetap pakai m.workspace, OS cwd tidak sync)", abs, err))
		return
	}
	m.appendLocal("TELEPORT", fmt.Sprintf("🪄 Workspace: %s → %s\n(tools now operate relative to new root; session continues)", old, abs))
}

// ─── /sandbox-toggle — toggle sandboxed execution ───────────────────────
func (m *model) handleSandboxToggle() {
	m.sandboxMode = !m.sandboxMode
	if m.sandboxMode {
		// Switch to plan mode as a simple sandbox
		tools.SetPermissionMode(tools.PermissionPlan)
		m.appendLocal("SANDBOX", "🔒 Sandbox ON — switched to plan mode (read-only). Semua write/exec di-block.\nUntuk kembali: /sandbox-toggle atau /perm default")
	} else {
		tools.SetPermissionMode(tools.PermissionDefault)
		m.appendLocal("SANDBOX", "🔓 Sandbox OFF — kembali ke default mode. Write/exec akan prompt konfirmasi.")
	}
}

// ─── Post-sampling hooks — extensible hook system ────────────────────────

// PostSamplingHook is a function called after each model response.
type PostSamplingHook func(response string, turnCount int)

var postSamplingHooks []PostSamplingHook

// RegisterPostSamplingHook adds a hook that fires after every model response.
func RegisterPostSamplingHook(hook PostSamplingHook) {
	postSamplingHooks = append(postSamplingHooks, hook)
}

// ExecutePostSamplingHooks runs all registered hooks with the model response.
func ExecutePostSamplingHooks(response string, turnCount int) {
	for _, hook := range postSamplingHooks {
		hook(response, turnCount)
	}
}

// ─── Session Memory — background note extraction ────────────────────────

// SessionMemory maintains running notes about the current session.
type SessionMemory struct {
	Notes     []string
	UpdatedAt time.Time
	FilePath  string
}

var globalSessionMemory *SessionMemory

// InitSessionMemory creates the session memory store.
func InitSessionMemory(sessionID string) *SessionMemory {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".flowork", "memories")
	_ = os.MkdirAll(dir, 0755)

	sm := &SessionMemory{
		Notes:     []string{},
		UpdatedAt: time.Now(),
		FilePath:  filepath.Join(dir, sessionID+".md"),
	}
	globalSessionMemory = sm

	// Register post-sampling hook for auto memory extraction
	RegisterPostSamplingHook(func(response string, turnCount int) {
		sm.extractNotes(response, turnCount)
	})

	return sm
}

// extractNotes extracts key information from a response and saves it.
func (sm *SessionMemory) extractNotes(response string, turnCount int) {
	// Extract key decisions, file changes, and important findings
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Auto-extract lines that look like decisions or key findings
		if strings.HasPrefix(trimmed, "✅") ||
			strings.HasPrefix(trimmed, "⚠️") ||
			strings.HasPrefix(trimmed, "🔴") ||
			strings.Contains(trimmed, "DECISION:") ||
			strings.Contains(trimmed, "IMPORTANT:") ||
			strings.Contains(trimmed, "NOTE:") {
			sm.Notes = append(sm.Notes, fmt.Sprintf("[turn %d] %s", turnCount, trimmed))
		}
	}
	sm.UpdatedAt = time.Now()
	sm.save()
}

// save persists the session memory to disk.
func (sm *SessionMemory) save() {
	if sm.FilePath == "" || len(sm.Notes) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString("# Session Memory\n\n")
	sb.WriteString(fmt.Sprintf("Updated: %s\n\n", sm.UpdatedAt.Format("2006-01-02 15:04:05")))
	for _, note := range sm.Notes {
		sb.WriteString("- " + note + "\n")
	}
	_ = os.WriteFile(sm.FilePath, []byte(sb.String()), 0644)
}

// GetSessionMemoryContext returns session notes for injection into prompt.
func GetSessionMemoryContext() string {
	if globalSessionMemory == nil || len(globalSessionMemory.Notes) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n<session-memory>\n")
	// Only include last 20 notes to avoid token bloat
	start := 0
	if len(globalSessionMemory.Notes) > 20 {
		start = len(globalSessionMemory.Notes) - 20
	}
	for _, note := range globalSessionMemory.Notes[start:] {
		sb.WriteString(note + "\n")
	}
	sb.WriteString("</session-memory>\n")
	return sb.String()
}

// ─── Extract Memories — durable cross-session memory ─────────────────────

// ExtractDurableMemories extracts key learnings and saves to project memory.
func ExtractDurableMemories(workspace string, events []string) {
	home, _ := os.UserHomeDir()
	memDir := filepath.Join(home, ".flowork", "memories", "projects")
	_ = os.MkdirAll(memDir, 0755)

	// Hash workspace path for filename
	projectName := filepath.Base(workspace)
	memPath := filepath.Join(memDir, projectName+".md")

	// Append new memories
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n## Session — %s\n\n", time.Now().Format("2006-01-02 15:04")))
	for _, event := range events {
		sb.WriteString("- " + event + "\n")
	}

	f, err := os.OpenFile(memPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(sb.String())
}

// ─── Tip System — contextual tips during thinking ────────────────────────

// Tip represents a contextual suggestion shown to the user.
type Tip struct {
	ID        string
	Category  string
	Message   string
	Condition func(turnCount int, toolName string) bool
}

var tipRegistry []Tip

func init() {
	// Register default tips — ported from Claude Code tipRegistry.ts
	tipRegistry = []Tip{
		{ID: "compact", Category: "performance", Message: "💡 Context window getting full? Use /compact --force to summarize and free space.", Condition: func(turns int, _ string) bool { return turns > 15 }},
		{ID: "think", Category: "quality", Message: "💡 For complex problems, try /think to enable deep reasoning mode.", Condition: func(turns int, _ string) bool { return turns == 3 }},
		{ID: "review", Category: "workflow", Message: "💡 Before committing, try /review to get an AI code review.", Condition: func(_ int, tool string) bool { return tool == "bash" }},
		{ID: "init", Category: "setup", Message: "💡 New project? Use /init to auto-generate your AGENTS.md.", Condition: func(turns int, _ string) bool { return turns == 1 }},
		{ID: "save", Category: "persistence", Message: "💡 Your session is auto-saved. Resume anytime with /resume --latest.", Condition: func(turns int, _ string) bool { return turns == 5 }},
		{ID: "stuck", Category: "help", Message: "💡 Stuck? Try /stuck for alternative approaches, or /rewind to roll back.", Condition: func(turns int, _ string) bool { return turns > 10 }},
		{ID: "perm", Category: "security", Message: "💡 Use /perm yolo to skip confirmation prompts (careful!), or /perm plan for read-only.", Condition: func(turns int, _ string) bool { return turns == 2 }},
		{ID: "cost", Category: "tracking", Message: "💡 Check your token usage and estimated cost with /cost.", Condition: func(turns int, _ string) bool { return turns%10 == 0 && turns > 0 }},
	}
}

// GetTip returns a contextual tip for the current state, or "" if none applies.
func GetTip(turnCount int, lastToolName string) string {
	for _, tip := range tipRegistry {
		if tip.Condition(turnCount, lastToolName) {
			return tip.Message
		}
	}
	return ""
}

// ─── Coordinator Mode — delegate work to sub-agents ──────────────────────

var coordinatorMode bool

// IsCoordinatorMode returns whether the agent is in coordinator mode.
func IsCoordinatorMode() bool {
	return coordinatorMode || os.Getenv("FLOWORK_COORDINATOR_MODE") == "1"
}

// SetCoordinatorMode toggles coordinator mode.
func SetCoordinatorMode(enabled bool) {
	coordinatorMode = enabled
}

// CoordinatorAllowedTools returns the restricted tool set for coordinator mode.
func CoordinatorAllowedTools() []string {
	return []string{"task", "task_parallel", "send_message", "team_create", "team_delete", "askuserquestion", "todo", "brief"}
}

// ─── Feature Gates — dynamic feature toggling ────────────────────────────

var featureGates = map[string]bool{
	"session_memory":   true,
	"post_sampling":    true,
	"tips":             true,
	"coordinator_mode": false,
	"extract_memories": true,
	"streaming_tui":    true,
}

// FeatureEnabled checks if a feature gate is enabled.
func FeatureEnabled(name string) bool {
	if v, ok := featureGates[name]; ok {
		return v
	}
	return false
}

// SetFeatureGate sets a feature gate value.
func SetFeatureGate(name string, enabled bool) {
	featureGates[name] = enabled
}

// ListFeatureGates returns all feature gates.
func ListFeatureGates() map[string]bool {
	result := make(map[string]bool)
	for k, v := range featureGates {
		result[k] = v
	}
	return result
}

// ─── Command Plugin Support — plugins can register slash commands ────────

// CommandPlugin represents a dynamically registered slash command from a plugin.
type CommandPlugin struct {
	Name        string
	Description string
	Handler     func(args []string) string
}

var commandPlugins []CommandPlugin

// RegisterCommandPlugin registers a plugin-provided slash command.
func RegisterCommandPlugin(plugin CommandPlugin) {
	commandPlugins = append(commandPlugins, plugin)
}

// LookupCommandPlugin finds a registered plugin command by name.
func LookupCommandPlugin(name string) *CommandPlugin {
	for i, p := range commandPlugins {
		if p.Name == name {
			return &commandPlugins[i]
		}
	}
	return nil
}

// ─── Shared Inbox (flowork_chat) ─────────────────────────────────────────

// SharedInbox provides a cross-agent message bus visible to all agents.
type SharedInbox struct {
	Channel  string
	Messages []SharedInboxMessage
}

// SharedInboxMessage is a single message in the shared inbox.
type SharedInboxMessage struct {
	From      string    `json:"from"`
	Content   string    `json:"content"`
	Channel   string    `json:"channel"`
	Timestamp time.Time `json:"timestamp"`
}

var globalSharedInbox = &SharedInbox{Channel: "main"}

// WriteToSharedInbox posts a message to the shared inbox.
func WriteToSharedInbox(from, message, channel string) {
	if channel == "" {
		channel = globalSharedInbox.Channel
	}
	globalSharedInbox.Messages = append(globalSharedInbox.Messages, SharedInboxMessage{
		From:      from,
		Content:   message,
		Channel:   channel,
		Timestamp: time.Now(),
	})
}

// ReadSharedInbox returns recent messages from the shared inbox.
func ReadSharedInbox(channel string, limit int) []SharedInboxMessage {
	if channel == "" {
		channel = globalSharedInbox.Channel
	}
	if limit <= 0 {
		limit = 20
	}
	var filtered []SharedInboxMessage
	for _, msg := range globalSharedInbox.Messages {
		if msg.Channel == channel {
			filtered = append(filtered, msg)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

// ─── Helpers ─────────────────────────────────────────────────────────────
