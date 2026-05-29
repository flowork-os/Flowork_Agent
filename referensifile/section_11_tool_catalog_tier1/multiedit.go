package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/teetah2402/flowork/internal/mempool"
	"github.com/teetah2402/flowork/internal/provider"
)

// MultiEditTool — apply multiple edits to a single file in sequence.
// Each edit is applied to the result of the previous. Atomic: if any edit
// fails, the file is left unchanged.
type MultiEditTool struct {
	root string
}

type multiEditArgs struct {
	Path  string         `json:"path"`
	Edits []multiEditOne `json:"edits"`
}

type multiEditOne struct {
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content" validate:"required"`
	All        bool   `json:"all,omitempty"`
}

func NewMultiEditTool(root string) *MultiEditTool {
	return &MultiEditTool{root: root}
}

func (t *MultiEditTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "multiedit",
		Description: "Apply multiple edits to a single file atomically. Each edit replaces old_content with new_content; if all=true, replace all occurrences. Edits applied sequentially.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to file (relative to workspace).",
				},
				"edits": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"old_content": map[string]any{"type": "string"},
							"new_content": map[string]any{"type": "string"},
							"all":         map[string]any{"type": "boolean"},
						},
						"required": []string{"old_content", "new_content"},
					},
				},
			},
			"required": []string{"path", "edits"},
		},
	}
}

func (t *MultiEditTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args multiEditArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode multiedit arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Path) == "" {
		return Result{}, fmt.Errorf("path is required")
	}
	if len(args.Edits) == 0 {
		return Result{}, fmt.Errorf("at least one edit required")
	}

	abs, err := SafeJoin(t.root, args.Path)
	if err != nil {
		return Result{}, err
	}

	// Gemini audit fix: use openNoFollow instead of os.ReadFile to close TOCTOU
	readFile, err := openNoFollow(abs, os.O_RDONLY, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("open file for read %s: %w", abs, err)
	}
	data, err := io.ReadAll(readFile)
	readFile.Close()
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", abs, err)
	}
	content := string(data)

	for i, e := range args.Edits {
		if e.OldContent == "" {
			return Result{}, fmt.Errorf("edit %d: old_content is empty", i+1)
		}
		if !strings.Contains(content, e.OldContent) {
			return Result{}, fmt.Errorf("edit %d: old_content not found", i+1)
		}
		if e.All {
			content = strings.ReplaceAll(content, e.OldContent, e.NewContent)
		} else {
			// First match only
			idx := strings.Index(content, e.OldContent)
			content = content[:idx] + e.NewContent + content[idx+len(e.OldContent):]
		}
	}

	editSizeSum := 0
	for _, e := range args.Edits {
		editSizeSum += len(e.NewContent)
	}

	diff := mempool.Diff{
		Author: "Flowork AI (MultiEditTool)",
		Reason: fmt.Sprintf("MultiEditTool: modifying %s", filepath.Base(abs)),
		Patches: []mempool.Patch{
			{Path: abs, After: content},
		},
	}
	res, handled, ctxErr := tryMempool(ctx, t.root, diff, editSizeSum)
	if handled {
		return res, ctxErr
	}

	writeFile, err := openNoFollow(abs, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("open file for write %s: %w", abs, err)
	}
	_, errWrite := writeFile.WriteString(content)
	writeFile.Close()
	if errWrite != nil {
		return Result{}, fmt.Errorf("write %s: %w", abs, errWrite)
	}

	return Result{
		Output: fmt.Sprintf("Applied %d edits to %s", len(args.Edits), args.Path),
		Metadata: map[string]any{
			"path":  args.Path,
			"edits": len(args.Edits),
		},
	}, nil
}
