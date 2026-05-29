package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// ReadTool menyediakan kemampuan membaca file, mendukung pembacaan per range nomor baris.
type ReadTool struct {
	root         string
	maxReadBytes int
}

type readArgs struct {
	Path     string `json:"path" validate:"required"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	MaxBytes int    `json:"max_bytes,omitempty"`
}

func NewReadTool(root string) *ReadTool {
	return &ReadTool{
		root:         root,
		maxReadBytes: 64 * 1024,
	}
}

// Definition mengembalikan definisi read tool yang terlihat oleh model.
func (t *ReadTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "read",
		Description: `Membaca file dari local filesystem. Dapat mengakses file mana pun.

Aturan pakai:
  - Path WAJIB absolute, bukan relative.
  - Default: baca sampai 2000 baris dari awal. Untuk file besar, kasih offset & limit — JANGAN baca seluruh file kalau kamu cuma butuh sebagian (boros context).
  - Hasil pakai format cat -n (nomor baris di kiri, mulai dari 1). Saat menyalin teks ke Edit, JANGAN ikutkan prefix nomor baris.
  - **JANGAN** pakai cat / head / tail lewat Bash. Pakai Read.
  - Untuk PDF besar (>10 hal), pakai parameter pages (mis. "1-5") — kalau tidak, akan error.
  - Tool ini cuma baca FILE, bukan directory. Untuk list directory pakai Glob (atau ls via Bash kalau perlu).

Owner override berlaku.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Workspace-relative path to the file.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Starting line number (1-indexed). Default is 1.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of lines to read. Default is all lines.",
				},
				"max_bytes": map[string]any{
					"type":        "integer",
					"description": "Maximum bytes to read. Default is 64KB.",
				},
			},
			"required": []string{"path"},
		},
	}
}

// Execute menjalankan operasi read file.
func (t *ReadTool) Execute(_ context.Context, invocation Invocation) (Result, error) {
	var args readArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode read arguments: %w", err)
	}
	if err := ValidateRequiredEdu(&args, t.root, "read"); err != nil {
		return Result{}, err
	}

	if strings.TrimSpace(args.Path) == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "read", "path"))
	}

	target, err := SafeJoin(t.root, args.Path)
	if err != nil {
		return Result{}, err
	}

	maxBytes := t.maxReadBytes
	if args.MaxBytes > 0 {
		maxBytes = args.MaxBytes
	}

	offset := args.Offset
	if offset < 0 {
		offset = 0
	}
	limit := args.Limit
	if limit < 0 {
		limit = 0
	}

	return t.readLines(target, offset, limit, maxBytes)
}

func (t *ReadTool) readLines(target string, offset, limit, maxBytes int) (Result, error) {
	// Gemini audit fix: use openNoFollow instead of os.Open to prevent TOCTOU
	// race condition where a symlink is swapped in after SafeJoin validation.
	file, err := openNoFollow(target, os.O_RDONLY, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_OPEN_FAILED", target), err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxBytes)

	var lines []string
	totalLines := 0
	truncated := false
	bytesRead := 0

	for scanner.Scan() {
		totalLines++
		line := scanner.Text()
		bytesRead += len(line) + 1

		if bytesRead > maxBytes {
			truncated = true
			break
		}

		if offset > 0 && totalLines < offset {
			continue
		}

		if limit > 0 && len(lines) >= limit {
			break
		}

		lines = append(lines, fmt.Sprintf("%6d\t%s", totalLines, line))
	}

	if err := scanner.Err(); err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_READ_FAILED", target), err)
	}

	output := strings.Join(lines, "\n")
	if truncated {
		output += "\n... (truncated)"
	}

	return Result{
		Output: output,
		Metadata: map[string]any{
			"path":        target,
			"total_lines": totalLines,
			"read_lines":  len(lines),
			"offset":      offset,
			"limit":       limit,
			"truncated":   truncated,
		},
	}, nil
}
