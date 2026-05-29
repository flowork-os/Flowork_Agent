// Package tools — memory_dir.go: Phase 3.3 Memory Directory.
//
// Adopt Claude Code CLAUDE.md style. File-based memory:
//   - ~/.flowork/memory/FLOWORK.md (global user memory)
//   - <project>/.flowork/MEMORY.md (project-scoped memory)
//
// Loaded sebagai system prompt prefix tiap session. User edit via slash
// /memory atau MemoryWrite tool.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/teetah2402/flowork/internal/provider"
)

// MemoryDirPath — resolve memory directory.
func MemoryDirPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flowork", "memory")
}

// GlobalMemoryFile — path to FLOWORK.md global memory.
func GlobalMemoryFile() string {
	dir := MemoryDirPath()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "FLOWORK.md")
}

// LoadGlobalMemory — read FLOWORK.md content. Empty kalau ngga ada.
func LoadGlobalMemory() string {
	p := GlobalMemoryFile()
	if p == "" {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(data)
}

// MemoryReadTool — read memory file.
type MemoryReadTool struct{}

func NewMemoryReadTool() *MemoryReadTool { return &MemoryReadTool{} }

func (t *MemoryReadTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "MemoryRead",
		Description: "Read global memory file ~/.flowork/memory/FLOWORK.md",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *MemoryReadTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	content := LoadGlobalMemory()
	if content == "" {
		return Result{
			Output: "Memory empty atau file ngga ada di " + GlobalMemoryFile(),
		}, nil
	}
	return Result{
		Output:   "# Global Memory (FLOWORK.md)\n\n" + content,
		Metadata: map[string]any{"path": GlobalMemoryFile(), "size": len(content)},
	}, nil
}

// MemoryWriteTool — append/replace memory file.
type MemoryWriteTool struct{}

type memoryWriteArgs struct {
	Content string `json:"content" validate:"required"`
	Append  bool   `json:"append,omitempty"`
}

func NewMemoryWriteTool() *MemoryWriteTool { return &MemoryWriteTool{} }

func (t *MemoryWriteTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "MemoryWrite",
		Description: "Write to global memory ~/.flowork/memory/FLOWORK.md. Append=true untuk add, false untuk replace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{"type": "string"},
				"append":  map[string]any{"type": "boolean", "description": "Append (default false = replace)"},
			},
			"required": []string{"content"},
		},
	}
}

func (t *MemoryWriteTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args memoryWriteArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("MemoryWrite: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("MemoryWrite: validation: %w", err)
	}
	p := GlobalMemoryFile()
	if p == "" {
		return Result{}, fmt.Errorf("MemoryWrite: ngga bisa resolve memory path")
	}
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	var content string
	if args.Append {
		existing, _ := os.ReadFile(p)
		content = string(existing) + "\n\n" + args.Content
	} else {
		content = args.Content
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		return Result{}, fmt.Errorf("MemoryWrite: write: %w", err)
	}
	return Result{
		Output: fmt.Sprintf("Memory updated %s (%d bytes, append=%v)", p, len(content), args.Append),
		Metadata: map[string]any{
			"path":   p,
			"size":   len(content),
			"append": args.Append,
		},
	}, nil
}
