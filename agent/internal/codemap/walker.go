// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package codemap

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "referensifile": true,
	"web": true, "bin": true, ".scratch": true, "sdk": true, "__pycache__": true,
}

type FileInfo struct {
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	FileType        string   `json:"file_type"`
	LineCount       int      `json:"line_count"`
	Layer           string   `json:"layer"`
	HasTests        bool     `json:"has_tests"`
	HasDocs         bool     `json:"has_docs"`
	HealthScore     int      `json:"health_score"`
	RecentlyTouched bool     `json:"recently_touched"`
	Issues          []string `json:"issues"`
}

type FileImport struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func WalkRepo(root string) ([]FileInfo, []FileImport, error) {
	root = filepath.Clean(root)
	type parsed struct {
		rel     string
		dir     string
		loc     int
		hasDocs bool
		imports []string
		touched bool
	}
	var files []parsed
	dirHasTest := map[string]bool{}
	dirGoFiles := map[string][]string{}

	goModRoots := map[string]string{}
	fset := token.NewFileSet()
	now := time.Now()

	err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] || strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Base(path) == "go.mod" {
			if mp := parseModulePath(path); mp != "" {
				if rd, e := filepath.Rel(root, filepath.Dir(path)); e == nil {
					goModRoots[mp] = filepath.Clean(rd)
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		dir := filepath.Dir(rel)
		if strings.HasSuffix(path, "_test.go") {
			dirHasTest[dir] = true
			return nil
		}
		content, cerr := os.ReadFile(path)
		if cerr != nil {
			return nil
		}
		loc := strings.Count(string(content), "\n") + 1
		hasDocs := false
		imports := []string{}
		if af, perr := parser.ParseFile(fset, path, content, parser.ParseComments); perr == nil {

			hasDocs = af.Doc != nil || len(af.Comments) > 0
			for _, imp := range af.Imports {

				imports = append(imports, strings.Trim(imp.Path.Value, `"`))
			}
		}
		dirGoFiles[dir] = append(dirGoFiles[dir], rel)
		files = append(files, parsed{
			rel: rel, dir: dir, loc: loc, hasDocs: hasDocs, imports: imports,
			touched: now.Sub(info.ModTime()) < 24*time.Hour,
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	rep := map[string]string{}
	for dir, fs := range dirGoFiles {
		sort.Strings(fs)
		want := filepath.Base(dir) + ".go"
		chosen := fs[0]
		for _, f := range fs {
			if filepath.Base(f) == want {
				chosen = f
				break
			}
		}
		rep[dir] = chosen
	}

	nodes := make([]FileInfo, 0, len(files))
	edges := []FileImport{}
	seenEdge := map[string]bool{}
	for _, f := range files {
		hasTest := dirHasTest[f.dir]
		score := 100
		issues := []string{}
		if f.loc > 500 {
			score -= 30
			issues = append(issues, "file > 500 LOC — pertimbangkan split (nano-modular)")
		}
		if f.loc > 800 {
			score -= 10
		}
		if !hasTest {
			score -= 15
			issues = append(issues, "paket tanpa _test.go")
		}
		if !f.hasDocs {
			score -= 10
			issues = append(issues, "tanpa komentar dokumentasi")
		}
		if score < 0 {
			score = 0
		}
		nodes = append(nodes, FileInfo{
			Path: f.rel, Name: filepath.Base(f.rel), FileType: "go",
			LineCount: f.loc, Layer: layerOf(f.rel), HasTests: hasTest,
			HasDocs: f.hasDocs, HealthScore: score, RecentlyTouched: f.touched,
			Issues: issues,
		})

		for _, imp := range f.imports {
			impDir := resolveImportDir(imp, goModRoots)
			if impDir == "" {
				continue
			}
			target, ok := rep[impDir]
			if !ok || target == f.rel {
				continue
			}
			key := f.rel + "→" + target
			if seenEdge[key] {
				continue
			}
			seenEdge[key] = true
			edges = append(edges, FileImport{From: f.rel, To: target})
		}
	}
	return nodes, edges, nil
}

func parseModulePath(goModPath string) string {
	b, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "module "))
		}
	}
	return ""
}

func resolveImportDir(imp string, roots map[string]string) string {
	best := ""
	for mp := range roots {
		if imp == mp || strings.HasPrefix(imp, mp+"/") {
			if len(mp) > len(best) {
				best = mp
			}
		}
	}
	if best == "" {
		return ""
	}
	sub := strings.TrimPrefix(strings.TrimPrefix(imp, best), "/")
	return filepath.Clean(filepath.Join(roots[best], sub))
}

func layerOf(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 1 {
		return "root"
	}
	if parts[0] == "internal" && len(parts) > 2 {
		return parts[1]
	}
	return parts[0]
}
