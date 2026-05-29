package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// GrepTool mencari teks yang cocok pada konten file.
type GrepTool struct {
	root string
}

type grepArgs struct {
	Path       string `json:"path"`
	Pattern    string `json:"pattern" validate:"required"`
	Glob       string `json:"glob,omitempty"`
	Regex      bool   `json:"regex,omitempty"`
	IgnoreCase bool   `json:"ignore_case,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
	Context    int    `json:"context,omitempty"`
}

func NewGrepTool(root string) *GrepTool {
	return &GrepTool{
		root: root,
	}
}

// Definition mengembalikan definisi grep tool yang terlihat oleh model.
func (t *GrepTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "grep",
		Description: `Content search tool — regex-aware, workspace-safe.

Aturan pakai:
  - **SELALU** pakai Grep untuk pencarian konten. **JANGAN** panggil grep / rg lewat Bash. Tool Grep ini sudah dioptimasi untuk permission rules dan output yang konsisten.
  - Untuk pencarian open-ended yang butuh banyak putaran (>3 query), pakai sub-agent (Task tool) — agar konteks utama tidak penuh hasil intermediate.
  - Filter file via 'glob' (mis. "*.go", "**/*.tsx", "src/**/*.go"). Pattern "**" matches at any depth.
  - Output adalah konten-match dengan format "path:line: text". Set 'context' untuk baris sebelum/sesudah match.

Owner override berlaku.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Workspace-relative path to search in. Default is '.'.",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "Text or regex pattern to search for.",
				},
				"glob": map[string]any{
					"type":        "string",
					"description": "File glob pattern to filter which files to search (e.g., '*.go', '*.md').",
				},
				"regex": map[string]any{
					"type":        "boolean",
					"description": "Treat pattern as regex. Default is false.",
				},
				"ignore_case": map[string]any{
					"type":        "boolean",
					"description": "Case-insensitive search. Default is false.",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matches to return. Default is 50.",
				},
				"context": map[string]any{
					"type":        "integer",
					"description": "Number of context lines before/after each match. Default is 0.",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

// Execute menjalankan pencarian grep.
func (t *GrepTool) Execute(_ context.Context, invocation Invocation) (Result, error) {
	var args grepArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode grep arguments: %w", err)
	}
	if err := ValidateRequiredEdu(&args, t.root, "grep"); err != nil {
		return Result{}, err
	}

	if strings.TrimSpace(args.Pattern) == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "grep", "pattern"))
	}

	searchPath := args.Path
	if strings.TrimSpace(searchPath) == "" {
		searchPath = "."
	}

	target, err := SafeJoin(t.root, searchPath)
	if err != nil {
		return Result{}, err
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	return t.grep(target, args.Pattern, args.Glob, args.Regex, args.IgnoreCase, maxResults, args.Context)
}

// matchGlob is a small glob matcher that extends filepath.Match with "**".
// CODEX-BUG-13 fix: `filepath.Match(pattern, filepath.Base(path))` ignored
// directory context, so patterns like "src/**/*.tsx" silently returned zero.
// Supported forms:
//   - plain basename glob:    "*.go"
//   - leading depth wildcard: "**/*.tsx"
//   - rooted depth wildcard:  "src/**/*.go"
//   - bare basename:          "README.md"
func matchGlob(pattern, relPath string) bool {
	if pattern == "" {
		return true
	}
	relPath = filepath.ToSlash(relPath)
	relPath = strings.TrimPrefix(relPath, "./")

	if !strings.Contains(pattern, "**") {
		m, _ := filepath.Match(pattern, filepath.Base(relPath))
		return m
	}

	// Split once at the first "**". parts[0] is the prefix (may end with "/"),
	// parts[1] is the suffix (may start with "/"). A trailing "/" or leading
	// "/" is normalised out so the anchor check is direction-agnostic.
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	if prefix != "" && !(relPath == prefix || strings.HasPrefix(relPath, prefix+"/")) {
		return false
	}
	if suffix == "" {
		return true
	}
	// Suffix without any "/" is a basename glob over the trailing segment.
	if !strings.Contains(suffix, "/") {
		m, _ := filepath.Match(suffix, filepath.Base(relPath))
		return m
	}
	// Suffix with "/" is matched as a literal tail (rare in grep use).
	return strings.HasSuffix(relPath, suffix)
}

func (t *GrepTool) grep(dir, pattern, fileGlob string, isRegex, ignoreCase bool, maxResults, ctxLines int) (Result, error) {
	var re *regexp.Regexp
	var err error

	if isRegex {
		if ignoreCase {
			pattern = "(?i)" + pattern
		}
		re, err = regexp.Compile(pattern)
		if err != nil {
			return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_PATTERN_INVALID", pattern, err.Error()), err)
		}
	}

	var results []string
	matchCount := 0

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" ||
				base == ".venv" || base == "__pycache__" || base == "dist" || base == ".next" {
				return filepath.SkipDir
			}
			// Gemini audit Bug 2.1 fix: skip traversal into sensitive dirs
			// (~/.flowork/keys, ~/.flowork/sessions, etc). Without this
			// check the interceptor only sees the top-level path arg, not
			// individual files that `grep -r` uncovers recursively.
			if isSensitivePath(t.root, path) {
				return filepath.SkipDir
			}
			return nil
		}

		if matchCount >= maxResults {
			return filepath.SkipAll
		}

		// Per-file sensitive check — blocks reading .env / owner.hash content
		// even when the search root (e.g. ".") is innocent.
		if isSensitivePath(t.root, path) {
			return nil
		}

		if fileGlob != "" {
			// CODEX-BUG-13: use matchGlob (handles "**") against the path
			// relative to the grep root so "src/**/*.tsx" works.
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				rel = filepath.Base(path)
			}
			if !matchGlob(fileGlob, rel) {
				return nil
			}
		}

		matches := t.searchFile(path, pattern, re, ignoreCase, dir, ctxLines)
		for _, m := range matches {
			if matchCount >= maxResults {
				break
			}
			results = append(results, m)
			matchCount++
		}

		return nil
	})

	if err != nil {
		return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_DIRECTORY_WALK_FAILED", dir), err)
	}

	output := strings.Join(results, "\n")
	if len(results) == 0 {
		output = "No matches found."
	}

	return Result{
		Output: output,
		Metadata: map[string]any{
			"path":    dir,
			"pattern": pattern,
			"count":   len(results),
		},
	}, nil
}

func (t *GrepTool) searchFile(filePath, pattern string, re *regexp.Regexp, ignoreCase bool, baseDir string, ctxLines int) []string {
	// Gemini audit fix: use openNoFollow instead of os.Open to prevent TOCTOU
	file, err := openNoFollow(filePath, os.O_RDONLY, 0o644)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// CODEX-BUG-14 fix: bufio.Scanner default token size is 64KB; minified JS
	// / JSON / bundle files blew past that silently, returning nil with
	// scanner.Err() set. Bump to 10 MiB per line which covers practical cases
	// while still rejecting pathological inputs explicitly below.
	const maxLine = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxLine)

	// When context is requested we keep a rolling ring-buffer of previous
	// lines and a countdown of how many trailing lines to still emit after
	// a match. Kept minimal — no merging of adjacent hits; behaviour mirrors
	// ripgrep's `-C` at a basic level.
	var (
		lines   []string
		lineNum int
		prev    []string
		tail    int
	)
	if ctxLines > 0 {
		prev = make([]string, 0, ctxLines)
	}

	relPath, relErr := filepath.Rel(baseDir, filePath)
	if relErr != nil {
		relPath = filePath
	}
	emit := func(prefix string, n int, text string) {
		lines = append(lines, fmt.Sprintf("%s:%d:%s %s", relPath, n, prefix, text))
	}
	plainEmit := func(n int, text string) {
		lines = append(lines, fmt.Sprintf("%s:%d: %s", relPath, n, text))
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		var matched bool
		if re != nil {
			matched = re.MatchString(line)
		} else if ignoreCase {
			matched = strings.Contains(strings.ToLower(line), strings.ToLower(pattern))
		} else {
			matched = strings.Contains(line, pattern)
		}

		switch {
		case matched && ctxLines == 0:
			plainEmit(lineNum, line)
		case matched:
			for i, p := range prev {
				emit("-", lineNum-len(prev)+i, p)
			}
			prev = prev[:0]
			emit(":", lineNum, line)
			tail = ctxLines
		case tail > 0:
			emit("-", lineNum, line)
			tail--
		default:
			if ctxLines > 0 {
				if len(prev) == ctxLines {
					prev = prev[1:]
				}
				prev = append(prev, line)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// CODEX-BUG-14 (observability half): surface rather than swallow. A
		// single entry keeps the existing result shape intact and signals to
		// caller/agent that the file was not fully analysed.
		lines = append(lines, fmt.Sprintf("%s:0: (warning) scan error: %v", relPath, err))
	}

	return lines
}
