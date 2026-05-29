// Package tools — compact_context.go: Phase 3.2 Context Compression.
//
// Adopt Claude Code /compact pattern + Hermes Agent session summarization.
// Compress long conversation context via LLM summary call, preserve sacred
// (doctrine + identity NEVER compacted).
//
// Trigger: manual via tool, atau auto saat context > threshold (Phase 5.x).

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// CompactContextTool — compress long conversation chunk.
type CompactContextTool struct{}

type compactArgs struct {
	Content       string `json:"content" validate:"required"`
	TargetTokens  int    `json:"target_tokens,omitempty"`
	PreserveFacts bool   `json:"preserve_facts,omitempty"`
}

func NewCompactContextTool() *CompactContextTool { return &CompactContextTool{} }

func (t *CompactContextTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "CompactContext",
		Description: "Compress long context via LLM summary. Preserve facts (numbers/names/dates) " +
			"+ key decisions, drop fluff/repetition. Sacred section NEVER compacted.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content":        map[string]any{"type": "string", "description": "Content to compress"},
				"target_tokens":  map[string]any{"type": "integer", "description": "Target output tokens (default ~500)"},
				"preserve_facts": map[string]any{"type": "boolean", "description": "Strict fact preservation (default true)"},
			},
			"required": []string{"content"},
		},
	}
}

func (t *CompactContextTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args compactArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("CompactContext: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("CompactContext: validation: %w", err)
	}
	if args.TargetTokens <= 0 {
		args.TargetTokens = 500
	}

	// Sacred markers — DETECT + REFUSE compact kalau content contain
	sacredMarkers := []string{
		"DOKTRIN SAKRAL", "SACRED HEADER", "AMP 999999",
		"Ayah Aola Sahidin", "heir whitelist", "DMS",
		"RULE 5 universal humane", "CSAM",
	}
	for _, m := range sacredMarkers {
		if strings.Contains(args.Content, m) {
			return Result{
				Output: "REFUSED: content contains sacred marker '" + m + "' — sacred section NEVER compacted. Strip sacred dulu sebelum compact.",
				Metadata: map[string]any{
					"refused": true,
					"marker":  m,
				},
			}, fmt.Errorf("sacred content refuse compact")
		}
	}

	// Phase 3.2 SKELETON: actual LLM call pending. For now return placeholder
	// dengan informasi yang LLM bisa pakai untuk self-compact.
	preview := args.Content
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return Result{
		Output: fmt.Sprintf(`# CompactContext (Phase 3.2 SKELETON)

Input length: %d chars
Target tokens: %d
Preview: %s

Real implementation pending: LLM summary call (router.Pick("default").Chat with summary prompt).
For now, agent lo summarize manually pakai skill 'compact' (bundled/skills/compact.md).

Procedure manual (untuk LLM lo follow):
1. Identify key facts (numbers, names, dates) — preserve verbatim
2. Drop fluff + repetition + hedging language
3. Output: 5-bullet markdown (top-level only, each ≤ 1 sentence)
4. Order by importance (most critical first)
`,
			len(args.Content), args.TargetTokens, preview),
		Metadata: map[string]any{
			"input_length":  len(args.Content),
			"target_tokens": args.TargetTokens,
			"phase":         "3.2-skeleton",
		},
	}, nil
}
