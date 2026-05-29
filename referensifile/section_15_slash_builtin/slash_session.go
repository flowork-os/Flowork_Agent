package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/session"
)

func (m *model) handleResumeCommand(args []string) {
	if m.sessionStore == nil {
		m.appendLocal("RESUME", "Session persistence not configured.")
		return
	}

	if len(args) > 0 && args[0] == "--latest" {
		latestID, err := m.sessionStore.Latest()
		if err != nil {
			m.appendLocal("RESUME", "No sessions found: "+err.Error())
			return
		}
		m.resumeSession(latestID)
		return
	}

	if len(args) > 0 {
		m.resumeSession(args[0])
		return
	}

	// Interactive picker — show last 20 sessions
	sessions, err := m.sessionStore.List()
	if err != nil {
		m.appendLocal("RESUME", "Error listing sessions: "+err.Error())
		return
	}
	if len(sessions) == 0 {
		m.appendLocal("RESUME", "No saved sessions found.")
		return
	}

	limit := 20
	if len(sessions) < limit {
		limit = len(sessions)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Last %d sessions:\n\n", limit))
	for i, s := range sessions[:limit] {
		tags := ""
		if len(s.Tags) > 0 {
			tags = " [" + strings.Join(s.Tags, ", ") + "]"
		}
		sb.WriteString(fmt.Sprintf("  %2d. %s  %-12s %q (%d turns)%s\n",
			i+1, s.StartedAt.Format("2006-01-02 15:04"), s.Provider, s.Title, s.TurnCount, tags))
	}
	sb.WriteString("\nResume: /resume <session-id> or /resume --latest")
	m.appendLocal("RESUME", sb.String())
}

func (m *model) resumeSession(id string) {
	snapshot, err := m.sessionStore.Load(id)
	if err != nil {
		// Try loading from history
		entries, err2 := m.sessionStore.LoadHistory(id)
		if err2 != nil {
			m.appendLocal("RESUME", fmt.Sprintf("Cannot load session %q: %s", id, err))
			return
		}
		m.session.Messages = session.ReconstructMessages(entries)
		m.session.ID = id
		m.session.TurnCount = len(entries) / 2 // rough estimate
		m.appendLocal("RESUME", fmt.Sprintf("Resumed session %q from history (%d messages). Conversation continues.", id, len(entries)))
		return
	}

	m.session.Messages = snapshot.Messages
	m.session.ID = id
	m.session.TurnCount = snapshot.TurnCount
	m.appendLocal("RESUME", fmt.Sprintf("Resumed session %q (%d messages, %d turns). Conversation continues.", id, len(snapshot.Messages), snapshot.TurnCount))
}

func (m *model) handleSessionsListCommand(args []string) {
	if m.sessionStore == nil {
		m.appendLocal("SESSIONS", "Session persistence not configured.")
		return
	}

	var sessions []session.SessionMeta
	var err error

	// Check for --tag filter
	tagFilter := ""
	for i, arg := range args {
		if arg == "--tag" && i+1 < len(args) {
			tagFilter = args[i+1]
			break
		}
	}

	if tagFilter != "" {
		sessions, err = m.sessionStore.ListByTag(tagFilter)
	} else {
		sessions, err = m.sessionStore.List()
	}

	if err != nil {
		m.appendLocal("SESSIONS", "Error: "+err.Error())
		return
	}

	if len(sessions) == 0 {
		if tagFilter != "" {
			m.appendLocal("SESSIONS", fmt.Sprintf("No sessions with tag %q.", tagFilter))
		} else {
			m.appendLocal("SESSIONS", "No saved sessions.")
		}
		return
	}

	var sb strings.Builder
	if tagFilter != "" {
		sb.WriteString(fmt.Sprintf("Sessions tagged %q:\n\n", tagFilter))
	} else {
		sb.WriteString(fmt.Sprintf("%d session(s):\n\n", len(sessions)))
	}

	for _, s := range sessions {
		tags := ""
		if len(s.Tags) > 0 {
			tags = " [" + strings.Join(s.Tags, ", ") + "]"
		}
		sb.WriteString(fmt.Sprintf("  %s  %-12s %q (%d turns)%s\n    ID: %s\n",
			s.StartedAt.Format("2006-01-02 15:04"), s.Provider, s.Title, s.TurnCount, tags, s.ID))
	}
	m.appendLocal("SESSIONS", sb.String())
}

func (m *model) handleExportCommand(args []string) {
	if m.sessionStore == nil {
		m.appendLocal("EXPORT", "Session persistence not configured.")
		return
	}

	format := "md"
	outputPath := ""
	forShare := false

	for i, arg := range args {
		switch arg {
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
			}
		case "--path":
			if i+1 < len(args) {
				outputPath = args[i+1]
			}
		case "--for-share":
			forShare = true
		default:
			// no-op — exhaustive switch guard
		}
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("flowork-export-%s.%s", time.Now().Format("20060102-150405"), format)
	}

	var err error
	if forShare {
		err = m.sessionStore.ExportRedacted(m.session.ID, outputPath)
	} else if format == "json" {
		err = m.sessionStore.ExportJSON(m.session.ID, outputPath)
	} else {
		err = m.sessionStore.ExportMarkdown(m.session.ID, outputPath)
	}

	if err != nil {
		m.appendLocal("EXPORT", "Error: "+err.Error())
		return
	}
	m.appendLocal("EXPORT", fmt.Sprintf("Exported to %s (format: %s, redacted: %v)", outputPath, format, forShare))
}

func (m *model) handleRewindCommand(args []string) {
	if m.sessionStore == nil {
		m.appendLocal("REWIND", "Session persistence not configured.")
		return
	}

	sessionDir := m.sessionStore.SessionDir(m.session.ID)

	if len(args) > 0 && args[0] == "--session" {
		restored, err := session.RewindSession(sessionDir)
		if err != nil {
			m.appendLocal("REWIND", "Error: "+err.Error())
			return
		}
		m.appendLocal("REWIND", fmt.Sprintf("Restored %d files:\n  %s", len(restored), strings.Join(restored, "\n  ")))
		return
	}

	if len(args) == 0 {
		// List files with snapshots
		files, err := session.ListFileSnapshots(sessionDir)
		if err != nil || len(files) == 0 {
			m.appendLocal("REWIND", "No file snapshots in this session.\nUsage: /rewind <file> or /rewind --session")
			return
		}
		m.appendLocal("REWIND", fmt.Sprintf("Files with snapshots:\n  %s\n\nUsage: /rewind <file>", strings.Join(files, "\n  ")))
		return
	}

	// Rewind specific file
	if err := session.RewindFile(sessionDir, args[0]); err != nil {
		m.appendLocal("REWIND", "Error: "+err.Error())
		return
	}
	m.appendLocal("REWIND", fmt.Sprintf("Restored %s from snapshot.", args[0]))
}

func (m *model) finalizeSession() {
	if m.sessionStore == nil {
		return
	}
	// Save final state
	_ = m.sessionStore.SnapshotState(m.session.ID, m.session.Messages, m.session.TurnCount)
	meta := session.SessionMeta{
		ID:        m.session.ID,
		StartedAt: m.session.StartedAt,
		Provider:  m.provider,
		Title:     m.session.Title,
		Tags:      m.session.Tags,
		TurnCount: m.session.TurnCount,
		Model:     m.modelName,
	}
	_ = m.sessionStore.Finalize(meta, m.workspace)
}
