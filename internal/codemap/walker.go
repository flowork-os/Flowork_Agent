// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: File-level codemap indexer (graph tab). go/parser imports-only,
//   skip-dirs noise, health/issue heuristik, import→edge representatif.
//   E2E verified (139 node + 122 edge dari repo, anti-traversal).
//
// walker.go — file-level codemap indexer untuk GUI graph tab.
//
// Beda dari goparser.go (yang extract simbol func/type per file untuk
// tool warga), walker.go menghasilkan node LEVEL-FILE + edge import antar
// file — sesuai kontrak frontend codemap.js (D3 force graph):
//
//	node: {path, name, file_type, line_count, layer, has_tests, has_docs,
//	       health_score, issues[], recently_touched}
//	edge: {from, to}  (file importer → file representatif paket yang di-import)
//
// Root index = working dir server (repo Flowork). Skip folder noise.
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

// modulePrefix — import internal repo (dari go.mod `module flowork-gui`).
const modulePrefix = "flowork-gui/"

// skipDirs — folder yang ngga di-index (build artifact, reference, vendor).
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "referensifile": true,
	"web": true, "bin": true, ".scratch": true, "sdk": true, "__pycache__": true,
}

// FileInfo — satu node file.
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

// FileImport — edge file→file.
type FileImport struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// WalkRepo index semua .go di root → file node + import edge.
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
	dirHasTest := map[string]bool{}        // dir → ada _test.go
	dirGoFiles := map[string][]string{}     // dir → rel .go files (non-test)
	fset := token.NewFileSet()
	now := time.Now()

	err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return nil // skip unreadable, jangan abort
		}
		if info.IsDir() {
			if skipDirs[info.Name()] || strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
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
			return nil // test file ngga jadi node, cuma penanda
		}
		content, cerr := os.ReadFile(path)
		if cerr != nil {
			return nil
		}
		loc := strings.Count(string(content), "\n") + 1
		hasDocs := false
		imports := []string{}
		if af, perr := parser.ParseFile(fset, path, content, parser.ParseComments); perr == nil {
			// has_docs = ada package doc atau komentar apa pun di file.
			hasDocs = af.Doc != nil || len(af.Comments) > 0
			for _, imp := range af.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				if strings.HasPrefix(p, modulePrefix) {
					imports = append(imports, strings.TrimPrefix(p, modulePrefix))
				}
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

	// Representatif per dir (paket) untuk target edge: file == last segment dir, else pertama.
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
		// Edges: file → representatif paket yang di-import.
		for _, impDir := range f.imports {
			target, ok := rep[filepath.Clean(impDir)]
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

// layerOf — klasifikasi layer dari path (top-level dir, internal/X → X).
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
