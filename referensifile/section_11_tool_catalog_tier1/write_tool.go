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

// WriteTool menyediakan kemampuan menulis file, mendukung create, overwrite, dan append.
type WriteTool struct {
	root string
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content" validate:"required"`
	Append  bool   `json:"append,omitempty"`
	Create  bool   `json:"create,omitempty"`
}

func NewWriteTool(root string) *WriteTool {
	return &WriteTool{
		root: root,
	}
}

// Definition mengembalikan definisi write tool yang terlihat oleh model.
func (t *WriteTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "write",
		Description: "Write content to a file. Creates parent directories automatically. Supports overwrite (default) or append mode.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Workspace-relative path to the file.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file.",
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "Append to the file instead of replacing it. Default is false.",
				},
				"create": map[string]any{
					"type":        "boolean",
					"description": "Create the file if it doesn't exist. Default is true.",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

// wrapLockError detect lock-related error pattern (Windows: "being used by
// another process"/"share violation"; Linux: "text file busy"/"resource busy")
// dan swap error message dengan ERR_FILE_LOCKED edukatif. Pesan teknis asli
// di-append biar debugging tetap mungkin.
//
// Realisasi roadmap_ai_external.md skenario 5 — gesekan antar saudara warga AI.
func (t *WriteTool) wrapLockError(target string, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	lockPatterns := []string{
		"being used by another process", // Windows: process still has file open
		"share violation",                // Windows: open with sharing flag conflict
		"text file busy",                 // Linux: ETXTBSY (executable open)
		"resource busy",                  // Linux: EBUSY
		"cannot access the file",         // Windows: access denied (often lock)
	}
	for _, p := range lockPatterns {
		if strings.Contains(msg, p) {
			edu := braindb.GetEducationalError(t.root, "ERR_FILE_LOCKED", target)
			return fmt.Errorf("%s\n\n[teknis: %w]", edu, err)
		}
	}
	return err
}

// Execute menjalankan operasi write file.
func (t *WriteTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args writeArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode write arguments: %w", err)
	}
	if err := ValidateRequiredEdu(&args, t.root, "write"); err != nil {
		return Result{}, err
	}

	if strings.TrimSpace(args.Path) == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "write", "path"))
	}

	target, err := SafeJoin(t.root, args.Path)
	if err != nil {
		return Result{}, err
	}

	create := true
	if invocation.ParsedArgs != nil {
		if v, ok := invocation.ParsedArgs["create"].(bool); ok {
			create = v
		}
	}

	return t.write(ctx, target, args.Content, args.Append, create)
}

func (t *WriteTool) write(ctx context.Context, target string, content string, appendMode, create bool) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_FILE_WRITE_FAILED", target), err)
	}

	finalContent := content
	if appendMode {
		// BUG-016 fix: previous os.ReadFile follow symlink → TOCTOU bocor isi
		// /etc/passwd ke diff yang di-forward ke mempool/Healer. Pakai
		// openNoFollow biar O_NOFOLLOW reject symlink di syscall level —
		// konsisten dengan read_tool, edit_tool, grep_tool, multiedit yang
		// sudah harden via pattern sama.
		existingFile, err := openNoFollow(target, os.O_RDONLY, 0o644)
		if err == nil {
			existing, readErr := io.ReadAll(existingFile)
			existingFile.Close()
			if readErr == nil {
				finalContent = string(existing) + content
			}
		}
	}

	diff := mempool.Diff{
		Author: "Flowork AI (WriteTool)",
		Reason: fmt.Sprintf("WriteTool: modifying %s", filepath.Base(target)),
		Patches: []mempool.Patch{
			{Path: target, After: finalContent},
		},
	}
	res, handled, err := tryMempool(ctx, t.root, diff, len(content))
	if handled {
		return res, err
	}

	// gemini_bug_2 #23 fix: SafeJoin only protects against `..` traversal
	// on the *workspace-relative path*, it does NOT follow symlinks on the
	// actual filesystem. An attacker who can plant `ln -s /etc/passwd ok.txt`
	// inside the workspace used to trick write into clobbering the symlink
	// target. Check the leaf with os.Lstat first and refuse if it is a
	// symlink — users that legitimately want to overwrite a symlink can
	// rm it first. We also pass O_NOFOLLOW on platforms that support it
	// (via openNoFollow below) so the race between Lstat and OpenFile is
	// closed at the syscall level.
	if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_SYMLINK_ATTACK", target))
	}

	flag := os.O_WRONLY
	if create {
		flag |= os.O_CREATE
	}
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	file, err := openNoFollow(target, flag, 0o644)
	if err != nil {
		return Result{}, t.wrapLockError(target, fmt.Errorf("open file %q: %w", target, err))
	}
	defer file.Close()

	written, err := file.WriteString(content)
	if err != nil {
		return Result{}, t.wrapLockError(target, fmt.Errorf("write file %q: %w", target, err))
	}

	action := "wrote"
	if appendMode {
		action = "appended"
	}

	return Result{
		Output: fmt.Sprintf("%s %d bytes to %s", action, written, target),
		Metadata: map[string]any{
			"path":    target,
			"bytes":   written,
			"append":  appendMode,
			"created": create,
		},
	}, nil
}
