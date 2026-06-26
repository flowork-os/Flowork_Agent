// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

var fileCategoryWhitelist = map[string]struct{}{
	"tools":    {},
	"job":      {},
	"document": {},
	"media":    {},
	"cache":    {},
	"log":      {},
}

const maxFileBytes = 4 * 1024 * 1024

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

	safeName := filepath.Base(name)
	if safeName == "." || safeName == ".." || safeName == "/" {
		return "", fmt.Errorf("invalid name after sanitize")
	}
	sharedDir := tools.FromSharedDir(ctx)
	if sharedDir == "" {
		return "", fmt.Errorf("shared workspace not in context")
	}
	abs := filepath.Join(sharedDir, category, safeName)

	if !strings.HasPrefix(abs, sharedDir+string(os.PathSeparator)) && abs != sharedDir {
		return "", fmt.Errorf("resolved path escapes shared dir")
	}
	return abs, nil
}

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

	abs, rel, err := resolveFileArgs(ctx, args)
	if err != nil {
		return tools.Result{}, err
	}

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
