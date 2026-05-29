package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/mempool"
	"github.com/teetah2402/flowork/internal/provider"
)

// EditTool menyediakan kemampuan edit konten file secara presisi.
type EditTool struct {
	root string
}

type editArgs struct {
	Path       string `json:"path"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content" validate:"required"`
	All        bool   `json:"all,omitempty"`
}

func NewEditTool(root string) *EditTool {
	return &EditTool{
		root: root,
	}
}

// Definition mengembalikan definisi edit tool yang terlihat oleh model.
func (t *EditTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "edit",
		Description: `Lakukan exact string replacement di file.

WAJIB:
  - Pakai tool Read minimal sekali di percakapan ini SEBELUM edit. Tool ini akan error kalau kamu coba edit tanpa Read dulu.
  - Saat menyalin teks dari output Read, preserve indentation persis (tab/spasi) seperti yang muncul SETELAH prefix nomor baris. Format prefix: nomor + tab. Yang setelah tab adalah konten asli — jangan ikutkan prefix ke old_content.
  - LEBIH SUKA edit file existing daripada bikin file baru. JANGAN bikin file baru kecuali benar-benar diperlukan.
  - JANGAN bikin file dokumentasi (*.md) atau README kecuali owner minta eksplisit.
  - Hanya pakai emoji kalau owner minta. Default: tanpa emoji.
  - Edit GAGAL kalau old_content tidak unik di file. Solusinya: kasih lebih banyak konteks sekitar agar unik, atau pakai 'all: true' untuk replace semua kemunculan.

Owner override: kalau owner suruh bikin file baru / edit tanpa Read / pakai emoji, lakukan.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Workspace-relative path to the file.",
				},
				"old_content": map[string]any{
					"type":        "string",
					"description": "The exact content to find and replace. Must be unique in the file unless 'all' is true.",
				},
				"new_content": map[string]any{
					"type":        "string",
					"description": "The content to replace old_content with.",
				},
				"all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences of old_content. Default is false.",
				},
			},
			"required": []string{"path", "old_content", "new_content"},
		},
	}
}

// Execute menjalankan operasi edit file.
func (t *EditTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args editArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode edit arguments: %w", err)
	}
	if err := ValidateRequiredEdu(&args, t.root, "edit"); err != nil {
		return Result{}, err
	}

	if strings.TrimSpace(args.Path) == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "edit", "path"))
	}

	if args.OldContent == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "edit", "old_content"))
	}

	target, err := SafeJoin(t.root, args.Path)
	if err != nil {
		return Result{}, err
	}

	return t.edit(ctx, target, args.OldContent, args.NewContent, args.All)
}

func (t *EditTool) edit(ctx context.Context, target, oldContent, newContent string, all bool) (Result, error) {
	// Gemini audit fix: use openNoFollow instead of os.ReadFile to close TOCTOU
	readFile, err := openNoFollow(target, os.O_RDONLY, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_OPEN_FAILED", target), err)
	}
	data, err := io.ReadAll(readFile)
	readFile.Close()
	if err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_READ_FAILED", target), err)
	}

	content := string(data)
	count := strings.Count(content, oldContent)

	if count == 0 {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_EDIT_TARGET_NOT_FOUND", oldContent, target))
	}

	if !all && count > 1 {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_EDIT_AMBIGUOUS_MATCH", oldContent, count, target))
	}

	var newFileContent string
	if all {
		newFileContent = strings.ReplaceAll(content, oldContent, newContent)
	} else {
		newFileContent = strings.Replace(content, oldContent, newContent, 1)
	}

	diff := mempool.Diff{
		Author: "Flowork AI (EditTool)",
		Reason: fmt.Sprintf("EditTool: modifying %s", filepath.Base(target)),
		Patches: []mempool.Patch{
			{Path: target, After: newFileContent},
		},
	}
	res, handled, ctxErr := tryMempool(ctx, t.root, diff, len(newContent))
	if handled {
		return res, ctxErr
	}

	writeFile, err := openNoFollow(target, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_OPEN_FAILED", target), err)
	}
	defer writeFile.Close()

	if _, err := writeFile.WriteString(newFileContent); err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_WRITE_FAILED", target), err)
	}

	replacedCount := count
	if !all {
		replacedCount = 1
	}

	return Result{
		Output: fmt.Sprintf("replaced %d occurrence(s) in %s", replacedCount, target),
		Metadata: map[string]any{
			"path":           target,
			"replaced_count": replacedCount,
			"total_found":    count,
			"all":            all,
		},
	}, nil
}
