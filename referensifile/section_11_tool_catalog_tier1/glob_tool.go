package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/fsutil"
	"github.com/teetah2402/flowork/internal/provider"
)

// GlobTool mencari file berdasarkan pola nama file.
type GlobTool struct {
	root string
}

type globArgs struct {
	Path       string `json:"path"`
	Pattern    string `json:"pattern" validate:"required"`
	MaxResults int    `json:"max_results,omitempty"`
}

func NewGlobTool(root string) *GlobTool {
	return &GlobTool{
		root: root,
	}
}

// Definition mengembalikan definisi glob tool yang terlihat oleh model.
func (t *GlobTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "glob",
		Description: `Pencarian file pattern (cepat, segala ukuran codebase).

Aturan pakai:
  - **SELALU** pakai Glob untuk mencari file by pattern. **JANGAN** pakai find / ls lewat Bash.
  - Mendukung pattern: "*.go", "**/*.tsx", "src/**/*.{ts,js}", "?", "[abc]".
  - Hasil di-sort by modification time (terbaru duluan) — berguna untuk "file yang baru-baru ini diubah".
  - Untuk eksplorasi codebase yang butuh banyak langkah glob+grep, delegasikan ke sub-agent (Task tool).

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
					"description": "Glob pattern (e.g., '*.go', '**/*.txt', 'src/**/*.js').",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results. Default is 100.",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

// Execute menjalankan pencarian glob.
func (t *GlobTool) Execute(_ context.Context, invocation Invocation) (Result, error) {
	var args globArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode glob arguments: %w", err)
	}
	if err := ValidateRequiredEdu(&args, t.root, "glob"); err != nil {
		return Result{}, err
	}

	if strings.TrimSpace(args.Pattern) == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "glob", "pattern"))
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
		maxResults = 100
	}

	return t.glob(target, args.Pattern, maxResults)
}

func (t *GlobTool) glob(dir, pattern string, maxResults int) (Result, error) {
	// If pattern contains **, use Walk for recursive matching.
	// filepath.Glob does not support ** (treats it same as *).
	var matches []string
	if strings.Contains(pattern, "**") {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				base := info.Name()
				if base == ".git" || base == "node_modules" || base == "vendor" || base == ".venv" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				return nil
			}
			matched, matchErr := doubleStarMatch(pattern, filepath.ToSlash(rel))
			if matchErr == nil && matched {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_DIRECTORY_WALK_FAILED", dir), err)
		}
	} else {
		fullPattern := fsutil.SafeJoin(dir, pattern)
		var err error
		matches, err = filepath.Glob(fullPattern)
		if err != nil {
			return Result{}, fmt.Errorf("%s\n\n[teknis: %w]", braindb.GetEducationalError(t.root, "ERR_PATTERN_INVALID", pattern, err.Error()), err)
		}
	}

	sort.Strings(matches)
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		relPath, err := filepath.Rel(dir, match)
		if err != nil {
			relPath = match
		}
		info, err := fsutil.SafeStat(match)
		suffix := ""
		size := ""
		if err == nil {
			if info.IsDir() {
				suffix = "/"
			}
			size = fmt.Sprintf(" (%d bytes)", info.Size())
		}
		lines = append(lines, relPath+suffix+size)
	}

	return Result{
		Output: strings.Join(lines, "\n"),
		Metadata: map[string]any{
			"path":    dir,
			"pattern": pattern,
			"count":   len(lines),
		},
	}, nil
}

// doubleStarMatch matches a slash-separated path against a pattern that may
// contain ** (matches any number of path segments) as well as standard * and ?.
func doubleStarMatch(pattern, path string) (bool, error) {
	// Split pattern and path into segments.
	patParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	return matchSegments(patParts, pathParts)
}

func matchSegments(pat, path []string) (bool, error) {
	for len(pat) > 0 && len(path) > 0 {
		p := pat[0]
		if p == "**" {
			// ** matches zero or more path segments.
			pat = pat[1:]
			if len(pat) == 0 {
				return true, nil // ** at end matches everything remaining
			}
			// Try matching the rest of pat against every suffix of path.
			for i := 0; i <= len(path); i++ {
				if ok, err := matchSegments(pat, path[i:]); err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		}
		matched, err := filepath.Match(p, path[0])
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
		pat = pat[1:]
		path = path[1:]
	}
	// Consume trailing ** wildcards.
	for len(pat) > 0 && pat[0] == "**" {
		pat = pat[1:]
	}
	return len(pat) == 0 && len(path) == 0, nil
}
