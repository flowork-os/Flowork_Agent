package codeindex

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// JSFileInfo hasil parse satu file .js/.ts
type JSFileInfo struct {
	Imports         []string // path yang di-import (relatif atau absolute)
	ExportedSymbols []string // nama exported
	DocComment      string   // JSDoc comment pertama di file
	LineCount       int
	SizeBytes       int64
}

var (
	// import ... from '...' / import('...')
	reImportFrom   = regexp.MustCompile(`(?m)import\s+(?:[^'"]*from\s+)?['"]([^'"]+)['"]`)
	reRequire      = regexp.MustCompile(`(?m)require\(['"]([^'"]+)['"]\)`)
	// export function/const/class/let/var Name
	reExportNamed  = regexp.MustCompile(`(?m)^export\s+(?:default\s+)?(?:async\s+)?(?:function|class|const|let|var)\s+(\w+)`)
	// export { Name1, Name2 }
	reExportBraces = regexp.MustCompile(`(?m)export\s*\{([^}]+)\}`)
	// /** ... */ JSDoc block
	reJSDoc        = regexp.MustCompile(`(?s)/\*\*(.+?)\*/`)
)

// ParseJSFile parse satu file .js/.ts/.mjs
func ParseJSFile(absPath string) (*JSFileInfo, error) {
	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	src := string(data)

	info := &JSFileInfo{
		SizeBytes: fi.Size(),
		LineCount: strings.Count(src, "\n") + 1,
	}

	// JSDoc comment pertama
	if m := reJSDoc.FindStringSubmatch(src); len(m) > 1 {
		// Bersihkan leading " * "
		lines := strings.Split(m[1], "\n")
		cleaned := []string{}
		for _, l := range lines {
			l = strings.TrimSpace(l)
			l = strings.TrimPrefix(l, "* ")
			l = strings.TrimPrefix(l, "*")
			if l != "" {
				cleaned = append(cleaned, l)
			}
		}
		info.DocComment = strings.Join(cleaned, "\n")
	}

	// Imports — deduplicate
	seen := map[string]bool{}
	addImport := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			info.Imports = append(info.Imports, p)
		}
	}
	for _, m := range reImportFrom.FindAllStringSubmatch(src, -1) {
		if len(m) > 1 {
			addImport(m[1])
		}
	}
	for _, m := range reRequire.FindAllStringSubmatch(src, -1) {
		if len(m) > 1 {
			addImport(m[1])
		}
	}

	// Exported symbols
	seenSym := map[string]bool{}
	addSym := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seenSym[s] {
			seenSym[s] = true
			info.ExportedSymbols = append(info.ExportedSymbols, s)
		}
	}
	for _, m := range reExportNamed.FindAllStringSubmatch(src, -1) {
		if len(m) > 1 {
			addSym(m[1])
		}
	}
	for _, m := range reExportBraces.FindAllStringSubmatch(src, -1) {
		if len(m) > 1 {
			for _, name := range strings.Split(m[1], ",") {
				// handle "Name as Alias" → take original
				parts := strings.Fields(name)
				if len(parts) > 0 {
					addSym(parts[0])
				}
			}
		}
	}

	return info, nil
}

// ResolveJSImportToPath coba resolve import path relatif ke absolute path.
// Import yang mulai dengan './' atau '../' adalah relatif.
// Import tanpa prefix (bare specifier) = external dep → diabaikan.
func ResolveJSImportToPath(importPath, callerDir string) string {
	if !strings.HasPrefix(importPath, ".") {
		return "" // external / bare specifier
	}
	resolved := filepath.Join(callerDir, importPath)
	// Coba tambah ekstensi kalau belum ada
	exts := []string{"", ".js", ".ts", ".mjs", "/index.js", "/index.ts"}
	for _, ext := range exts {
		candidate := resolved + ext
		if _, err := os.Stat(candidate); err == nil {
			return filepath.ToSlash(candidate)
		}
	}
	// Kembalikan path apa adanya (mungkin folder → indexer handle)
	return filepath.ToSlash(resolved)
}
