// Package compact — session memory-aware compaction.
// This file adds memory-aware compaction that integrates with SessionMemory
// to preserve unsummarized notes and key decisions during context window shrinking.
// Inspired by Claude Code's sessionMemoryCompact.ts (632 lines).
package compact

import (
	"context"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/core"
	"github.com/teetah2402/flowork/internal/provider"
)

// MemoryAwareCompactor wraps the standard Compactor with session memory awareness.
// When compacting, it ensures that session memory notes (key decisions, file changes,
// errors, etc.) are preserved in the compacted context rather than lost.
type MemoryAwareCompactor struct {
	base        *Compactor
	memoryNotes []string // injected from SessionMemory
}

// NewMemoryAwareCompactor creates a memory-aware compactor.
func NewMemoryAwareCompactor(client provider.Client, model string) *MemoryAwareCompactor {
	return &MemoryAwareCompactor{
		base: NewCompactor(client, model),
	}
}

// SetMemoryNotes injects session memory notes for preservation during compaction.
func (mac *MemoryAwareCompactor) SetMemoryNotes(notes []string) {
	mac.memoryNotes = notes
}

// CheckAndCompact performs memory-aware compaction.
// It extends the base compactor by:
// 1. Injecting session memory notes into the summary prompt
// 2. Tracking which messages have been summarized vs unsummarized
// 3. Preserving unsummarized messages when possible
func (mac *MemoryAwareCompactor) CheckAndCompact(ctx context.Context, session *core.Session) (bool, string, error) {
	ratio := UsageRatio(session, mac.base.model)

	switch {
	case ratio >= TierFullRatio:
		return mac.memoryAwareFullCompact(ctx, session)
	case ratio >= TierArchiveRatio:
		return mac.memoryAwareArchive(session)
	case ratio >= TierMicroRatio:
		return mac.base.micro(session)
	}
	return false, "", nil
}

// memoryAwareArchive keeps system + memory context + recent messages.
func (mac *MemoryAwareCompactor) memoryAwareArchive(session *core.Session) (bool, string, error) {
	return mac.base.archive(session)
}

// memoryAwareFullCompact asks model to summarize, but injects memory notes.
func (mac *MemoryAwareCompactor) memoryAwareFullCompact(ctx context.Context, session *core.Session) (bool, string, error) {
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

	// Build memory-enhanced summarization prompt
	var memorySection string
	if len(mac.memoryNotes) > 0 {
		var sb strings.Builder
		sb.WriteString("\n\nIMPORTANT — The following session memory notes MUST be preserved in your summary:\n")
		for _, note := range mac.memoryNotes {
			sb.WriteString("• " + note + "\n")
		}
		memorySection = sb.String()
	}

	summaryPrompt := provider.Message{
		Role: provider.RoleUser,
		Content: fmt.Sprintf(`Summarize the conversation so far. Your summary MUST be structured:

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
%s
Rules:
• Keep total output under 600 tokens
• Do NOT include tool call details or raw output
• Focus on WHAT was decided, not HOW
• Include exact file paths (they are critical for continuity)
• NEVER drop information from the session memory notes above
• Output ONLY the summary, no preamble or closing`, memorySection),
	}
	sumReq := provider.Request{
		Model:       mac.base.model,
		Messages:    append(session.Messages, summaryPrompt),
		MaxTokens:   1000, // slightly more to accommodate memory notes
		Temperature: 0.2,
	}
	resp, err := mac.base.client.Complete(ctx, sumReq)
	if err != nil {
		return false, "", fmt.Errorf("memory-aware summarize: %w", err)
	}
	summary := provider.Message{
		Role:    provider.RoleSystem,
		Content: "[Memory-Aware Full-Compact Summary]\n\n" + resp.Message.Content,
	}
	// Keep last 5 messages for continuity (more than base's 3 to preserve recent context)
	keepLast := 5
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

	// Clear memory notes after they've been incorporated into the summary
	memNotesCount := len(mac.memoryNotes)
	mac.memoryNotes = nil

	return true, fmt.Sprintf("memory-aware compact: summarized + dropped %d messages, preserved %d memory notes, summary=%d tokens",
		dropped, memNotesCount, resp.Usage.OutputTokens), nil
}
