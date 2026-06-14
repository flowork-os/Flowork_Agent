// file_path_resolver.go — NON-LOCKED extension. Adds a Claude-style `file_path`
// (relative, workspace-confined) as an ALTERNATIVE to {category, name} for the file
// tools (file_read / file_write / edit). Owner-approved (2026-06-14): keeps the model
// vocabulary native to the distilled traces + an external Claude driver, which already
// speak file_path.
//
// ISOLATION PRESERVED (Opus 4.6's invariant intact): file_path is cleaned, then
// rejected if absolute / has a Windows drive / contains a `..` escape, then resolved
// UNDER tools.FromSharedDir(ctx) with a post-Join HasPrefix escape check — exactly the
// same containment as validateCategoryAndName. The ONLY relaxation is that nested
// subdirectories are allowed INSIDE the agent's isolated workspace (vs basename-flat).
// The original {category, name} path stays the fallback, byte-for-byte unchanged.
package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/tools"
)

// resolveFileArgs resolves a file tool's target, preferring `file_path` (relative,
// workspace-confined) and falling back to {category, name}. Returns (absPath, relForReport).
func resolveFileArgs(ctx context.Context, args map[string]any) (string, string, error) {
	if fp, ok := args["file_path"].(string); ok && strings.TrimSpace(fp) != "" {
		return resolveWorkspaceRel(ctx, fp)
	}
	cat, _ := args["category"].(string)
	name, _ := args["name"].(string)
	abs, err := validateCategoryAndName(ctx, cat, name)
	if err != nil {
		return "", "", err
	}
	return abs, strings.TrimSpace(cat) + "/" + filepath.Base(strings.TrimSpace(name)), nil
}

// resolveWorkspaceRel maps a relative file_path to an absolute path under the agent's
// isolated workspace, rejecting any attempt to escape it.
func resolveWorkspaceRel(ctx context.Context, fp string) (string, string, error) {
	fp = strings.ReplaceAll(strings.TrimSpace(fp), "\\", "/")
	if fp == "" {
		return "", "", fmt.Errorf("file_path required")
	}
	// Reject absolute (unix) + Windows drive (C:\…): an isolated agent has no host fs.
	if filepath.IsAbs(fp) || (len(fp) >= 2 && fp[1] == ':') {
		return "", "", fmt.Errorf("[PETUNJUK, bukan salahmu] file_path harus RELATIF di dalam workspace-mu, bukan absolut %q — tiap agent terisolasi di foldernya sendiri (doktrin ERR_WORKSPACE_ESCAPE)", fp)
	}
	clean := filepath.Clean(fp)
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", "", fmt.Errorf("[PETUNJUK, bukan salahmu] file_path %q nembus keluar workspace (pakai '..') — diblok demi isolasi (doktrin ERR_WORKSPACE_ESCAPE)", fp)
	}
	shared := tools.FromSharedDir(ctx)
	if shared == "" {
		return "", "", fmt.Errorf("shared workspace not in context")
	}
	abs := filepath.Join(shared, clean)
	// Defense in depth — resolved abs must stay under the workspace.
	if !strings.HasPrefix(abs, shared+string(os.PathSeparator)) && abs != shared {
		return "", "", fmt.Errorf("file_path escapes workspace")
	}
	return abs, clean, nil
}
