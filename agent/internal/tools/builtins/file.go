// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Section 11 phase 1b (file ops 3 tools) DONE. API stable:
//   file_read/file_write/file_list. Path safety via category whitelist
//   (tools/job/document/media/cache/log) + filepath.Base strip
//   traversal + defense-in-depth HasPrefix check post-Join. 4MB content
//   cap. Symlink skip di file_list (audit Section 6 pattern). Phase 1c+
//   add edit/glob/grep → tambah file baru (mis. `file_advanced.go`),
//   JANGAN modify ini.
//
// file.go — Section 11 phase 1b: 3 file ops tools.
//
// Tools:
//   1. file_read   — read file in shared workspace category
//   2. file_write  — write file (create or overwrite)
//   3. file_list   — list filenames in category
//
// SECURITY:
//   - Path NEVER raw from user — caller pass {category, name}.
//   - category WHITELIST (mirror SharedSubfolders di kernelhost).
//   - name sanitized via filepath.Base — strip any slashes/dots.
//   - Resolved path: <shared_dir>/<category>/<name>.
//   - Read/write cap 4MB content.
//
// REGISTRATION:
//   `Init()` di builtins.go panggil register5 demo + register3 file
//   (overall 8 tools). Phase 1c/1d/etc tambah lebih banyak.

package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowork-gui/internal/tools"
)

// fileCategoryWhitelist — mirror kernelhost.SharedSubfolders.
var fileCategoryWhitelist = map[string]struct{}{
	"tools":    {},
	"job":      {},
	"document": {},
	"media":    {},
	"cache":    {},
	"log":      {},
}

const maxFileBytes = 4 * 1024 * 1024 // 4MB cap

// validateCategoryAndName — safe path resolver. Return (absPath, error).
// Sanity: category whitelist, name via filepath.Base (anti-traversal).
func validateCategoryAndName(ctx context.Context, category, name string) (string, error) {
	category = strings.TrimSpace(category)
	name = strings.TrimSpace(name)
	if category == "" {
		return "", fmt.Errorf("category required")
	}
	if _, ok := fileCategoryWhitelist[category]; !ok {
		return "", fmt.Errorf("category %q not in whitelist (tools/job/document/media/cache/log)", category)
	}
	if name == "" {
		return "", fmt.Errorf("name required")
	}
	// filepath.Base strips any path traversal attempts.
	safeName := filepath.Base(name)
	if safeName == "." || safeName == ".." || safeName == "/" {
		return "", fmt.Errorf("invalid name after sanitize")
	}
	sharedDir := tools.FromSharedDir(ctx)
	if sharedDir == "" {
		return "", fmt.Errorf("shared workspace not in context")
	}
	abs := filepath.Join(sharedDir, category, safeName)
	// Defense in depth — ensure resolved abs masih di bawah sharedDir.
	if !strings.HasPrefix(abs, sharedDir+string(os.PathSeparator)) && abs != sharedDir {
		return "", fmt.Errorf("resolved path escapes shared dir")
	}
	return abs, nil
}

// =============================================================================
// file_read — read file content
// =============================================================================

type fileReadTool struct{}

func (fileReadTool) Name() string       { return "file_read" }
func (fileReadTool) Capability() string { return "fs:read:/shared/*" }
func (fileReadTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Read a file from your workspace. Preferred: file_path (relative path inside your workspace, e.g. 'docs/notes.md'). Legacy: category + name. 4MB cap.",
		Params: []tools.Param{
			{Name: "file_path", Type: tools.ParamString, Description: "relative path in your workspace (preferred), e.g. 'src/main.go'. Absolute/'..' rejected (isolation)."},
			{Name: "category", Type: tools.ParamString, Description: "legacy: tools|job|document|media|cache|log (use file_path instead)"},
			{Name: "name", Type: tools.ParamString, Description: "legacy: filename (basename only) — pair with category"},
		},
		Returns: "{path, content, size_bytes, truncated: bool}",
	}
}
func (fileReadTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	// file_path (relative, workspace-confined) preferred; {category,name} fallback.
	abs, rel, err := resolveFileArgs(ctx, args)
	if err != nil {
		return tools.Result{}, err
	}
	info, serr := os.Stat(abs)
	if serr != nil {
		if os.IsNotExist(serr) {
			return tools.Result{}, fmt.Errorf("file not found: %s", rel)
		}
		return tools.Result{}, fmt.Errorf("stat: %w", serr)
	}
	if info.IsDir() {
		return tools.Result{}, fmt.Errorf("path is directory, not file")
	}
	truncated := false
	readLimit := info.Size()
	if readLimit > maxFileBytes {
		readLimit = maxFileBytes
		truncated = true
	}
	f, oerr := os.Open(abs)
	if oerr != nil {
		return tools.Result{}, fmt.Errorf("open: %w", oerr)
	}
	defer f.Close()
	buf := make([]byte, readLimit)
	n, rerr := f.Read(buf)
	if rerr != nil && rerr.Error() != "EOF" {
		return tools.Result{}, fmt.Errorf("read: %w", rerr)
	}
	return tools.Result{Output: map[string]any{
		"path":       rel,
		"content":    string(buf[:n]),
		"size_bytes": info.Size(),
		"truncated":  truncated,
	}}, nil
}

// =============================================================================
// file_write — write file (create or overwrite)
// =============================================================================

type fileWriteTool struct{}

func (fileWriteTool) Name() string       { return "file_write" }
func (fileWriteTool) Capability() string { return "fs:write:/shared/*" }
func (fileWriteTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Write a file in your workspace (create or overwrite). Preferred: file_path (relative path, e.g. 'src/util.go' — parent dirs auto-created). Legacy: category + name. Content cap 4MB.",
		Params: []tools.Param{
			{Name: "file_path", Type: tools.ParamString, Description: "relative path in your workspace (preferred). Absolute/'..' rejected (isolation)."},
			{Name: "content", Type: tools.ParamString, Description: "file content", Required: true},
			{Name: "category", Type: tools.ParamString, Description: "legacy: tools|job|document|media|cache|log (use file_path instead)"},
			{Name: "name", Type: tools.ParamString, Description: "legacy: filename (basename only) — pair with category"},
		},
		Returns: "{path, bytes_written}",
	}
}
func (fileWriteTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return tools.Result{}, fmt.Errorf("content required")
	}
	if len(content) > maxFileBytes {
		return tools.Result{}, fmt.Errorf("content > %d bytes cap", maxFileBytes)
	}
	// file_path (relative, workspace-confined) preferred; {category,name} fallback.
	abs, rel, err := resolveFileArgs(ctx, args)
	if err != nil {
		return tools.Result{}, err
	}
	// Ensure parent dir exists (file_path may nest new subdirs in the workspace).
	if mkerr := os.MkdirAll(filepath.Dir(abs), 0o755); mkerr != nil {
		return tools.Result{}, fmt.Errorf("mkdir: %w", mkerr)
	}
	if werr := os.WriteFile(abs, []byte(content), 0o644); werr != nil {
		return tools.Result{}, fmt.Errorf("write: %w", werr)
	}
	return tools.Result{Output: map[string]any{
		"path":          rel,
		"bytes_written": len(content),
	}}, nil
}

// =============================================================================
// file_list — list filenames in category
// =============================================================================

type fileListTool struct{}

func (fileListTool) Name() string       { return "file_list" }
func (fileListTool) Capability() string { return "fs:read:/shared/*" }
func (fileListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List filenames in shared workspace category. Symlinks skipped.",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "tools|job|document|media|cache|log", Required: true},
		},
		Returns: "{category, files: [string], count: int}",
	}
}
func (fileListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	cat, _ := args["category"].(string)
	cat = strings.TrimSpace(cat)
	if cat == "" {
		return tools.Result{}, fmt.Errorf("category required")
	}
	if _, ok := fileCategoryWhitelist[cat]; !ok {
		return tools.Result{}, fmt.Errorf("category %q not in whitelist", cat)
	}
	sharedDir := tools.FromSharedDir(ctx)
	if sharedDir == "" {
		return tools.Result{}, fmt.Errorf("shared workspace not in context")
	}
	dir := filepath.Join(sharedDir, cat)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.Result{Output: map[string]any{
				"category": cat, "files": []string{}, "count": 0,
			}}, nil
		}
		return tools.Result{}, fmt.Errorf("readdir: %w", err)
	}
	files := []string{}
	for _, e := range entries {
		// Skip symlinks for safety (anti symlink follow leak).
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return tools.Result{Output: map[string]any{
		"category": cat,
		"files":    files,
		"count":    len(files),
	}}, nil
}
