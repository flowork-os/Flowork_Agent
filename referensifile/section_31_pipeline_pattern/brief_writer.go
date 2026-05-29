package api

// ╔══════════════════════════════════════════════════════════════════════╗
// ║ 🔒 SACRED LOCK — Brief Writer (Curated Context, Anti Over-Prompt)    ║
// ║                                                                      ║
// ║ Dibangun Mr.Dev + Mr.Flow 2026-05-26.                                ║
// ║ Pattern dari Architect Agent Blueprint:                              ║
// ║   "runs after every section, prepares curated context brief for      ║
// ║    next agent; extracts Founder Voice verbatim for tone-sensitive    ║
// ║    sections."                                                        ║
// ║                                                                      ║
// ║ Fix problem real: v19 OOM 36K tokens > 32K context window.           ║
// ║ Brief Writer compress session history jadi summary, kurangi prompt.  ║
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

// Brief Writer constants — tuned untuk Qwen3-8B 32K context.
const (
	// MaxBriefLen — max length curated brief. 800 char = ~200 tokens.
	MaxBriefLen = 800

	// CompressThreshold — kalau total history > N char, compress jadi brief.
	// 8000 char = ~2000 tokens. Trigger compress kalau sebelum reach 32K total.
	CompressThreshold = 8000

	// RecentTurnsKeep — turn terakhir N keep verbatim, sisanya summary.
	// 2 = last user + last assistant kept full, older compressed.
	RecentTurnsKeep = 2
)

// CompressHistoryToBrief — kalau history terlalu panjang, ringkas turn lama
// jadi 1 brief summary, keep N turn terakhir verbatim.
//
// Pattern:
//   [verbatim full] system prompt
//   [BRIEF] turn 1-N: ringkasan poin penting (max 800 char)
//   [verbatim full] turn (N-K+1) sampai N (user+assistant)
//   [verbatim full] current user message
//
// Return: compressed messages list. Total reduction ~70-90% untuk session
// panjang.
//
// Caller (warga.Process) check len(history) > CompressThreshold sebelum
// invoke. Brief writer ini stateless, deterministic — ngga LLM-driven,
// just text extraction (anti-cost).
func CompressHistoryToBrief(messages []types.Message) []types.Message {
	if len(messages) <= RecentTurnsKeep+1 {
		// Too short, no compression needed
		return messages
	}

	// Calculate total length
	totalLen := 0
	for _, m := range messages {
		totalLen += len(m.Content)
	}
	if totalLen < CompressThreshold {
		return messages
	}

	// Strategy: keep system + last 2*RecentTurnsKeep messages verbatim,
	// compress middle into brief.
	systemIdx := -1
	for i, m := range messages {
		if strings.EqualFold(m.Role, "system") {
			systemIdx = i
			break
		}
	}

	keepFromEnd := 2 * RecentTurnsKeep
	if len(messages)-systemIdx-1 <= keepFromEnd {
		return messages // not enough middle to compress
	}

	// Build brief from middle messages (everything between system and last K turns)
	middleStart := systemIdx + 1
	middleEnd := len(messages) - keepFromEnd
	if middleEnd <= middleStart {
		return messages
	}

	briefBuilder := strings.Builder{}
	briefBuilder.WriteString("[brief — ringkasan turn sebelumnya]\n")
	for i := middleStart; i < middleEnd; i++ {
		m := messages[i]
		// Extract topic + first-sentence summary
		summary := extractKeyPoint(m.Content)
		fmt.Fprintf(&briefBuilder, "%s: %s\n", m.Role, summary)
	}
	briefBuilder.WriteString("[end brief]")
	briefContent := briefBuilder.String()
	if len(briefContent) > MaxBriefLen {
		briefContent = briefContent[:MaxBriefLen] + "...[truncated]"
	}

	// Reconstruct: [system, brief, last K turns verbatim]
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

// extractKeyPoint — pull first sentence (or first 120 char) from content.
// Stateless deterministic extraction, anti-LLM cost.
func extractKeyPoint(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	// Find first period or newline
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

// CountTotalTokensEstimate — rough estimate total token (3.5 char per token).
// Untuk pre-check sebelum LLM call. Caller compare vs ctxSize.
func CountTotalTokensEstimate(messages []types.Message) int {
	totalChar := 0
	for _, m := range messages {
		totalChar += len(m.Content)
		// Add overhead untuk role tags
		totalChar += 10
	}
	return totalChar / 3 // conservative — Indonesian biasanya 3-4 char/token
}
