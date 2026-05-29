// Package compact provides automatic conversation compaction strategies.
// Triggers when session approaches context window limit to prevent crash.
package compact

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/teetah2402/flowork/internal/core"
	"github.com/teetah2402/flowork/internal/provider"
)

// safeKeepOffset finds the safe start index so that we never begin a kept
// window in the middle of an assistant tool-call / tool-result pair.
// If messages[start] is a RoleTool result, walk forward until we find a
// non-tool message (the tool results belong to the preceding assistant turn).
func safeKeepOffset(messages []provider.Message, start int) int {
	for start < len(messages) && messages[start].Role == provider.RoleTool {
		start++
	}
	return start
}

// truncateRunes — UTF-8 safe truncate (prevents mid-codepoint slicing).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// Tier thresholds (fraction of model context limit).
const (
	TierMicroRatio   = 0.70 // strip old tool outputs
	TierArchiveRatio = 0.85 // archive oldest N turns
	TierFullRatio    = 0.95 // ask model to summarize all
)

// ModelContextLimit returns approximate token capacity per model family.
//
// BUG-M15 fix (2026-04-19): tambah model baru (Claude 4.6/4.7 Opus/Sonnet,
// GPT-5, o1/o3, Gemini 2.5 Pro, DeepSeek v3, Llama 4, Qwen3) ke switch
// supaya compaction gak trigger prematur di 32k default. Default juga
// dinaikkan ke 128k — model modern mayoritas udah >= 128k, 32k terlalu
// pesimistis buat model yang belum kita kenal secara eksplisit.
func ModelContextLimit(model string) int {
	m := strings.ToLower(model)
	switch {
	// Gemini — 2.0 / 2.5 / 3.0 family = 1M+, legacy = 32k.
	case strings.Contains(m, "gemini-3"), strings.Contains(m, "gemini-2.5"),
		strings.Contains(m, "gemini-2.0"), strings.Contains(m, "gemini-exp"):
		return 1_000_000
	case strings.Contains(m, "gemini"):
		return 32_000
	// Claude — 4.x Opus/Sonnet (4-5, 4-6, 4-7) = 1M; legacy 3.x Opus/Sonnet = 200k.
	case strings.Contains(m, "claude-opus-4-7"),
		strings.Contains(m, "claude-opus-4-6"),
		strings.Contains(m, "claude-opus-4-5"),
		strings.Contains(m, "claude-opus-4"),
		strings.Contains(m, "claude-sonnet-4-7"),
		strings.Contains(m, "claude-sonnet-4-6"),
		strings.Contains(m, "claude-sonnet-4-5"),
		strings.Contains(m, "claude-sonnet-4"),
		strings.Contains(m, "claude-haiku-4"),
		strings.Contains(m, "opus-4"), strings.Contains(m, "sonnet-4"):
		return 1_000_000
	case strings.Contains(m, "claude"):
		return 200_000
	// OpenAI — GPT-5 / o3 / o1 = 200k+; GPT-4.1/4o = 128k.
	case strings.Contains(m, "gpt-5"), strings.Contains(m, "o3-"),
		strings.Contains(m, "o1-pro"):
		return 200_000
	case strings.Contains(m, "gpt-4.1"), strings.Contains(m, "gpt-4o"),
		strings.Contains(m, "o1"):
		return 128_000
	// DeepSeek v3 = 128k, earlier = 64k.
	case strings.Contains(m, "deepseek-v3"), strings.Contains(m, "deepseek-r1"):
		return 128_000
	case strings.Contains(m, "deepseek"):
		return 64_000
	// Grok-3 / Grok-4 = 131k.
	case strings.Contains(m, "grok"):
		return 128_000
	// Llama 4 / Llama 3.3 = 128k.
	case strings.Contains(m, "llama-4"), strings.Contains(m, "llama-3.3"),
		strings.Contains(m, "llama-3.2"):
		return 128_000
	case strings.Contains(m, "llama"):
		return 32_000
	// Qwen 2.5 / 3 = 131k+.
	case strings.Contains(m, "qwen3"), strings.Contains(m, "qwen-3"),
		strings.Contains(m, "qwen2.5"), strings.Contains(m, "qwen-2.5"):
		return 128_000
	case strings.Contains(m, "mistral-large"), strings.Contains(m, "mixtral-8x22"):
		return 128_000
	}
	// Default raised 32k → 128k. Modern OpenRouter catalog is ~all >=128k.
	return 128_000
}

// EstimateTokens — rough estimate of token count in session.
// Uses ~3.5 chars/token (conservative blend of English ~4 and code ~3).
// Counts ALL message fields: Content, ToolCalls (name + args), ToolCallID, Name.
// Adds a 15% safety margin so compaction triggers before actual context overflow.
func EstimateTokens(session *core.Session) int {
	total := 0
	for _, m := range session.Messages {
		// Content (main body of message — can be very large for tool results)
		total += len(m.Content)*2/7 + 1 // ≈ /3.5, avoids float
		// Tool call fields
		for _, tc := range m.ToolCalls {
			total += len(tc.Name)*2/7 + 1
			total += len(tc.Arguments)*2/7 + 1
			total += len(tc.ID)*2/7 + 1
		}
		// Tool result metadata
		if m.ToolCallID != "" {
			total += len(m.ToolCallID)*2/7 + 1
		}
		if m.Name != "" {
			total += len(m.Name)*2/7 + 1
		}
	}
	// 15% safety margin — better to compact slightly early than crash on overflow.
	return total * 115 / 100
}

// UsageRatio returns fraction of context limit used.
func UsageRatio(session *core.Session, model string) float64 {
	used := EstimateTokens(session)
	limit := ModelContextLimit(model)
	if limit <= 0 {
		return 0
	}
	return float64(used) / float64(limit)
}

// maxSessionTokens returns the absolute token cap for session sliding window.
// Configurable via FLOWORK_MAX_SESSION_TOKENS env var (default 20000).
// B-1: keeps cost predictable regardless of model context window size.
func maxSessionTokens() int {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_MAX_SESSION_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 20_000
}

// Compactor — top-level compaction driver.
type Compactor struct {
	client provider.Client
	model  string
}

func NewCompactor(client provider.Client, model string) *Compactor {
	return &Compactor{client: client, model: model}
}

// CheckAndCompact — called before each turn. Triggers appropriate tier if over threshold.
// Returns (compacted, msg, err). msg describes what was done if compacted.
//
// B-1: sliding window checked FIRST (absolute token cap) so it fires even for
// large-context models like Gemini 2.5 Flash (1M ctx) where ratio-based tiers
// never trigger until 700K tokens — far too late for cost control.
func (c *Compactor) CheckAndCompact(ctx context.Context, session *core.Session) (bool, string, error) {
	// Tier 0 (B-1): absolute sliding window — fires before ratio tiers.
	if done, msg, err := c.sliding(session); done || err != nil {
		return done, msg, err
	}
	ratio := UsageRatio(session, c.model)
	switch {
	case ratio >= TierFullRatio:
		return c.fullCompact(ctx, session)
	case ratio >= TierArchiveRatio:
		return c.archive(session)
	case ratio >= TierMicroRatio:
		return c.micro(session)
	}
	return false, "", nil
}

// sliding — B-1 sliding window with absolute token cap.
// Drops oldest non-system messages until session fits within maxSessionTokens().
// Runs every turn so context stays bounded regardless of session length.
func (c *Compactor) sliding(session *core.Session) (bool, string, error) {
	limit := maxSessionTokens()
	if EstimateTokens(session) <= limit {
		return false, "", nil
	}
	n := len(session.Messages)
	sysCount := 0
	for _, m := range session.Messages {
		if m.Role == provider.RoleSystem {
			sysCount++
		} else {
			break
		}
	}
	// Walk backward accumulating tokens until we reach 80% of limit
	// (leaves headroom for the upcoming assistant turn).
	budget := limit * 80 / 100
	kept := 0
	tokens := 0
	for i := n - 1; i >= sysCount; i-- {
		m := session.Messages[i]
		mTok := len(m.Content)*2/7 + 1
		for _, tc := range m.ToolCalls {
			mTok += len(tc.Name)*2/7 + 1
			mTok += len(tc.Arguments)*2/7 + 1
		}
		if tokens+mTok > budget {
			break
		}
		tokens += mTok
		kept++
	}
	if kept >= n-sysCount {
		return false, "", nil
	}
	keepStart := safeKeepOffset(session.Messages, n-kept)
	dropped := keepStart - sysCount
	if dropped <= 0 {
		return false, "", nil
	}
	// B-2: summarize the dropped turns instead of a blank tombstone notice.
	droppedMsgs := session.Messages[sysCount:keepStart]
	notice := provider.Message{
		Role:    provider.RoleSystem,
		Content: summarizeDropped(droppedMsgs, dropped, limit, kept),
	}
	newMsgs := make([]provider.Message, 0, sysCount+1+(n-keepStart))
	newMsgs = append(newMsgs, session.Messages[:sysCount]...)
	newMsgs = append(newMsgs, notice)
	newMsgs = append(newMsgs, session.Messages[keepStart:]...)
	session.Messages = newMsgs
	return true, fmt.Sprintf("sliding-window: dropped %d messages, session now ~%d tokens", dropped, EstimateTokens(session)), nil
}

// summarizeDropped — B-2: condense dropped messages into a structured summary.
// No LLM call needed — simple text extraction keeps this fast and free.
// Max output ~1000 chars so the summary itself doesn't eat context budget.
func summarizeDropped(msgs []provider.Message, dropped, limit, kept int) string {
	const perMsgMax = 180 // chars per message snippet

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[B-2 context summary] %d old turns condensed (session capped at %d tokens, %d recent kept):\n", dropped, limit, kept))

	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			if t := strings.TrimSpace(m.Content); t != "" {
				sb.WriteString("User: ")
				sb.WriteString(truncateRunes(t, perMsgMax))
				sb.WriteByte('\n')
			}
		case provider.RoleAssistant:
			for _, tc := range m.ToolCalls {
				sb.WriteString("→ ")
				sb.WriteString(tc.Name)
				sb.WriteByte('\n')
			}
			if t := strings.TrimSpace(m.Content); t != "" {
				sb.WriteString("Asst: ")
				sb.WriteString(truncateRunes(t, perMsgMax))
				sb.WriteByte('\n')
			}
		case provider.RoleTool:
			if t := strings.TrimSpace(m.Content); t != "" {
				sb.WriteString("  └ ")
				sb.WriteString(truncateRunes(t, 80))
				sb.WriteByte('\n')
			}
		default:
			// no-op — exhaustive switch guard
		}
		// Hard cap: stop adding once summary exceeds ~1000 chars.
		if sb.Len() > 1000 {
			sb.WriteString("  ... (further turns omitted)\n")
			break
		}
	}
	return sb.String()
}

// Tier 1: strip tool outputs from messages > N turns old
func (c *Compactor) micro(session *core.Session) (bool, string, error) {
	n := len(session.Messages)
	if n < 10 {
		return false, "", nil
	}
	cutoff := n - 10 // keep last 10 messages fully intact
	stripped := 0
	for i := 0; i < cutoff; i++ {
		m := &session.Messages[i]
		if m.Role == provider.RoleTool && len(m.Content) > 200 {
			m.Content = truncateRunes(m.Content, 200) + "\n...(truncated by micro-compact)"
			stripped++
		}
	}
	if stripped == 0 {
		return false, "", nil
	}
	return true, fmt.Sprintf("micro-compact: stripped %d old tool outputs", stripped), nil
}

// Tier 2: keep system + last 10 messages, drop middle
func (c *Compactor) archive(session *core.Session) (bool, string, error) {
	n := len(session.Messages)
	if n < 14 {
		return false, "", nil
	}
	keepLast := 10
	// Preserve system messages at start
	sysCount := 0
	for _, m := range session.Messages {
		if m.Role == provider.RoleSystem {
			sysCount++
		} else {
			break
		}
	}
	if n-sysCount <= keepLast {
		return false, "", nil
	}
	summary := provider.Message{
		Role:    provider.RoleSystem,
		Content: fmt.Sprintf("[Archive-compact] %d earlier turns removed to save context. Focus on recent conversation.", n-sysCount-keepLast),
	}
	keepStart := safeKeepOffset(session.Messages, n-keepLast)
	newMsgs := make([]provider.Message, 0, sysCount+1+(n-keepStart))
	newMsgs = append(newMsgs, session.Messages[:sysCount]...)
	newMsgs = append(newMsgs, summary)
	newMsgs = append(newMsgs, session.Messages[keepStart:]...)
	dropped := n - len(newMsgs)
	session.Messages = newMsgs
	return true, fmt.Sprintf("archive-compact: dropped %d old messages, kept last %d", dropped, n-keepStart), nil
}

// ForceFullCompact — public entry point for the /compact --force slash command.
// Runs tier-3 summarization unconditionally regardless of usage ratio.
func (c *Compactor) ForceFullCompact(ctx context.Context, session *core.Session) (bool, string, error) {
	return c.fullCompact(ctx, session)
}

// Tier 3: ask model to summarize, replace history
func (c *Compactor) fullCompact(ctx context.Context, session *core.Session) (bool, string, error) {
	n := len(session.Messages)
	if n < 8 {
		return false, "", nil
	}
	sysCount := 0
	for _, m := range session.Messages {
		if m.Role == provider.RoleSystem {
			sysCount++
		} else {
			break
		}
	}
	// Build summarization request — optimized prompt for high-quality summaries
	// Inspired by Claude Code's sessionMemoryCompact approach.
	summaryPrompt := provider.Message{
		Role: provider.RoleUser,
		Content: `Summarize the conversation so far. Your summary MUST be structured:

## Current Task
What the user is currently working on (1-2 sentences).

## Key Decisions
- List each important decision made (max 8 items)

## Files Modified
- List all file paths that were created, edited, or deleted

## Errors & Fixes
- Any errors encountered and how they were resolved

## Pending Work
- What still needs to be done (open TODOs, unfinished tasks)

## Important Context
- Environment details, constraints, or preferences mentioned

Rules:
• Keep total output under 600 tokens
• Do NOT include tool call details or raw output
• Focus on WHAT was decided, not HOW
• Include exact file paths (they are critical for continuity)
• Output ONLY the summary, no preamble or closing`,
	}
	sumReq := provider.Request{
		Model:       c.model,
		Messages:    append(session.Messages, summaryPrompt),
		MaxTokens:   800,
		Temperature: 0.2,
	}
	resp, err := c.client.Complete(ctx, sumReq)
	if err != nil {
		return false, "", fmt.Errorf("summarize: %w", err)
	}
	summary := provider.Message{
		Role:    provider.RoleSystem,
		Content: "[Full-compact summary of prior conversation]\n\n" + resp.Message.Content,
	}
	// Keep last 3 messages for continuity, aligned to a safe pair boundary.
	keepLast := 3
	if n-sysCount < keepLast {
		keepLast = n - sysCount
	}
	keepStart := safeKeepOffset(session.Messages, n-keepLast)
	newMsgs := make([]provider.Message, 0, sysCount+1+(n-keepStart))
	newMsgs = append(newMsgs, session.Messages[:sysCount]...)
	newMsgs = append(newMsgs, summary)
	newMsgs = append(newMsgs, session.Messages[keepStart:]...)
	dropped := n - len(newMsgs)
	session.Messages = newMsgs
	return true, fmt.Sprintf("full-compact: summarized + dropped %d messages, summary=%d tokens", dropped, resp.Usage.OutputTokens), nil
}
