package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/memory"
	"github.com/teetah2402/flowork/internal/provider"
)

// MemorySetTool — agent tulis key-value ke persistent memory namespace.
type MemorySetTool struct{ workspace string }

func NewMemorySetTool(workspace string) *MemorySetTool { return &MemorySetTool{workspace} }

func (t *MemorySetTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "memory_set",
		Description: "Simpan key-value ke persistent memory. Memory bertahan lintas sesi. Gunakan namespace 'global' untuk ingatan bersama, atau nama agent untuk ingatan pribadi.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "Namespace memory, misal 'global', 'telegram', 'opus-2'"},
				"key":       map[string]any{"type": "string", "description": "Nama key"},
				"value":     map[string]any{"type": "string", "description": "Nilai yang disimpan"},
			},
			"required": []string{"namespace", "key", "value"},
		},
	}
}

func (t *MemorySetTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	ns, _ := inv.ParsedArgs["namespace"].(string)
	key, _ := inv.ParsedArgs["key"].(string)
	value, _ := inv.ParsedArgs["value"].(string)
	if ns == "" || key == "" {
		return Result{ToolName: inv.ToolName, OK: false, Output: "Error: namespace dan key wajib diisi"}, nil
	}
	store, err := memory.Open(t.workspace, ns)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("Error: %v", err)}, nil
	}
	store.Set(key, value)
	return Result{ToolName: inv.ToolName, OK: true, Output: fmt.Sprintf("✓ memory[%s][%s] = %q", ns, key, value)}, nil
}

// MemoryGetTool — agent baca semua key-value dari persistent memory namespace.
type MemoryGetTool struct{ workspace string }

func NewMemoryGetTool(workspace string) *MemoryGetTool { return &MemoryGetTool{workspace} }

func (t *MemoryGetTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "memory_get",
		Description: "Baca semua key-value dari namespace memory. Gunakan untuk recall ingatan lintas sesi.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string", "description": "Namespace memory, misal 'global', 'telegram', 'opus-2'"},
			},
			"required": []string{"namespace"},
		},
	}
}

func (t *MemoryGetTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	ns, _ := inv.ParsedArgs["namespace"].(string)
	if ns == "" {
		return Result{ToolName: inv.ToolName, OK: false, Output: "Error: namespace wajib diisi"}, nil
	}
	store, err := memory.Open(t.workspace, ns)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("Error: %v", err)}, nil
	}
	all := store.All()
	if len(all) == 0 {
		return Result{ToolName: inv.ToolName, OK: true, Output: fmt.Sprintf("memory[%s] kosong", ns)}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("memory[%s] (%d entries):\n", ns, len(all)))
	for k, e := range all {
		sb.WriteString(fmt.Sprintf("  %s = %q  (updated: %s)\n", k, e.Value, e.TS.Format("2006-01-02 15:04")))
	}
	return Result{ToolName: inv.ToolName, OK: true, Output: sb.String()}, nil
}

// MemoryDeleteTool — agent hapus key dari persistent memory.
type MemoryDeleteTool struct{ workspace string }

func NewMemoryDeleteTool(workspace string) *MemoryDeleteTool { return &MemoryDeleteTool{workspace} }

func (t *MemoryDeleteTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "memory_delete",
		Description: "Hapus key dari namespace memory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{"type": "string"},
				"key":       map[string]any{"type": "string"},
			},
			"required": []string{"namespace", "key"},
		},
	}
}

func (t *MemoryDeleteTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	ns, _ := inv.ParsedArgs["namespace"].(string)
	key, _ := inv.ParsedArgs["key"].(string)
	store, err := memory.Open(t.workspace, ns)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("Error: %v", err)}, nil
	}
	store.Delete(key)
	return Result{ToolName: inv.ToolName, OK: true, Output: fmt.Sprintf("✓ memory[%s][%s] dihapus", ns, key)}, nil
}
