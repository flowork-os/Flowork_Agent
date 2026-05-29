package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/teetah2402/flowork/internal/factmemory"
	"github.com/teetah2402/flowork/internal/provider"
)

// AstSearchTool mencari definisi struktur Go dan fungsionalitas dalam repository.
type AstSearchTool struct {
	workspace string
}

type astSearchArgs struct {
	Query string `json:"query" validate:"required"`
}

func NewAstSearchTool(workspace string) *AstSearchTool {
	return &AstSearchTool{workspace: workspace}
}

func (t *AstSearchTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "search_semantic_function",
		Description: `(Cursor-Killer API). Cari secara leksikal & semantik pemanggilan fungsi, struct, atau interface berbekal nama fungsinya dalam repositori kode Go lokal. Jauh lebih cepat dan kuat daripada 'grep' untuk melacak kode.
Query bisa merupakan nama fungsi, nama tipe struct, atau nama package. Tool akan mengembalikan signature fungsi, filepath, dan lokasi baris jika ditemukan.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Nama fungsi/struct yang dicari (e.g., 'Execute', 'SandboxTool', or '(*Pool) runHealer')",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *AstSearchTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args astSearchArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }


	indexFile := filepath.Join(t.workspace, "state", "factmemory", "ast_index.json")

	// Jika belum ada file indeks, buat indeks dinamis sekarang.
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		if err := factmemory.BuildIndex(t.workspace); err != nil {
			return Result{}, fmt.Errorf("failed to build ast index: %w", err)
		}
	}

	b, err := os.ReadFile(indexFile)
	if err != nil {
		return Result{}, fmt.Errorf("read index: %w", err)
	}

	var index factmemory.ASTIndex
	if err := json.Unmarshal(b, &index); err != nil {
		return Result{}, fmt.Errorf("parse index: %w", err)
	}

	queryLower := strings.ToLower(args.Query)
	var matches []string

	for _, node := range index.Nodes {
		nameLower := strings.ToLower(node.Name)
		if strings.Contains(nameLower, queryLower) {

			matchStr := fmt.Sprintf("[%s] %s di %s:%d", node.Type, node.Name, node.Filepath, node.Line)
			if node.Signature != "" {
				matchStr += fmt.Sprintf("\n    Signature: %s", node.Signature)
			}
			matches = append(matches, matchStr)
		}
	}

	if len(matches) == 0 {
		return Result{
			Output: fmt.Sprintf("Tidak ada tipe/fungsi bernada '%s' yang ditemukan dalam ranah semantik Go.", args.Query),
		}, nil
	}

	if len(matches) > 50 {
		matches = matches[:50]
		matches = append(matches, "...(hasil dipotong pada elemen 50)")
	}

	return Result{
		Output: strings.Join(matches, "\n\n"),
	}, nil
}
