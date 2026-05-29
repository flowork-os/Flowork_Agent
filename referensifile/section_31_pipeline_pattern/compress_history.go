package warga

// ╔══════════════════════════════════════════════════════════════════════╗
// ║ 🔒 SACRED LOCK — Compress History (Brief Writer Pattern)             ║
// ║                                                                      ║
// ║ Dibangun Mr.Dev + Mr.Flow 2026-05-26.                                ║
// ║ Mirror dari kernel/api/brief_writer.go karena Go ngga support import ║
// ║ circular antara warga ↔ api.                                         ║
// ║                                                                      ║
// ║ Fix v19 OOM context (36K tokens > 32K). Compress history sebelum     ║
// ║ LLM call kalau total > threshold.                                    ║
// ║                                                                      ║
// ║ JANGAN MODIFIKASI tanpa baca:                                        ║
// ║   /home/mrflow/Documents/catatan_flowork/catatan                     ║
// ║   /home/mrflow/Documents/catatan_flowork/lock_file (SOP)             ║
// ║                                                                      ║
// ║ Bug? APPEND catatan + tunggu Mr.Dev konfirmasi.                      ║
// ╚══════════════════════════════════════════════════════════════════════╝

import (
	"fmt"
	"strings"

	"github.com/flowork/kernel/kernel/types"
)

const (
	briefMaxLen           = 800
	briefCompressTrigger  = 8000 // total char trigger
	briefRecentTurnsKeep  = 2
)

// CompressHistory — kalau llmMsgs total > trigger, ringkas turn lama
// jadi 1 brief, keep N turn terakhir verbatim.
func CompressHistory(messages []types.Message) []types.Message {
	if len(messages) <= briefRecentTurnsKeep+1 {
		return messages
	}
	totalLen := 0
	for _, m := range messages {
		totalLen += len(m.Content)
	}
	if totalLen < briefCompressTrigger {
		return messages
	}
	systemIdx := -1
	for i, m := range messages {
		if strings.EqualFold(m.Role, "system") {
			systemIdx = i
			break
		}
	}
	keepFromEnd := 2 * briefRecentTurnsKeep
	if len(messages)-systemIdx-1 <= keepFromEnd {
		return messages
	}
	middleStart := systemIdx + 1
	middleEnd := len(messages) - keepFromEnd
	if middleEnd <= middleStart {
		return messages
	}

	sb := strings.Builder{}
	sb.WriteString("[brief — ringkasan turn sebelumnya]\n")
	for i := middleStart; i < middleEnd; i++ {
		m := messages[i]
		summary := extractKeyPoint(m.Content)
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, summary)
	}
	sb.WriteString("[end brief]")
	briefContent := sb.String()
	if len(briefContent) > briefMaxLen {
		briefContent = briefContent[:briefMaxLen] + "...[truncated]"
	}

	out := make([]types.Message, 0, keepFromEnd+2)
	if systemIdx >= 0 {
		out = append(out, messages[:systemIdx+1]...)
	}
	out = append(out, types.Message{
		Role:    "system",
		Content: briefContent,
	})
	out = append(out, messages[middleEnd:]...)
	return out
}

func extractKeyPoint(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	for _, sep := range []string{". ", ".\n", "\n", "? ", "! "} {
		if idx := strings.Index(content, sep); idx > 10 && idx < 150 {
			return content[:idx+1]
		}
	}
	if len(content) > 120 {
		return content[:120] + "..."
	}
	return content
}
