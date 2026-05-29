//go:build ignore

// ext_path_safety_scanner — mendeteksi masalah keamanan path yang
// menyebabkan bug portabilitas & path traversal di multi-OS
// (Windows, Linux, macOS/Darwin).
//
// Scanner ini memeriksa:
//  1. File ops tanpa SafeJoin → path traversal (../../etc/passwd)
//  2. Hardcoded separator ("/" atau "\\") → gagal di OS lain
//  3. Import "path" bukan "path/filepath" → non-OS-aware
//  4. Hardcoded OS-specific paths (/tmp, C:\, /etc, /dev)
//  5. NTFS ADS attack vector (":"  di filename)
//  6. Windows reserved names (CON, PRN, NUL, etc)
//  7. Missing filepath.Clean/Abs sebelum security check
//  8. Symlink TOCTOU — os.Stat tanpa os.Lstat/EvalSymlinks
//
// Prinsip: FQP-9 (Gate Reversibility), GOL §B (Protected Core),
//
//	GOL §F (Gerbang Pertahanan)
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type PathFinding struct {
	Level   string // CRITICAL, HIGH, MEDIUM, LOW
	Type    string
	File    string
	Line    int
	Func    string
	Message string
}

var pathFindings []PathFinding

func main() {
	fmt.Println("🛤️  [EXT_PATH_SAFETY v1] Scanning for cross-OS path safety issues...")
	fmt.Println("   Target OS: Windows, Linux, macOS/Darwin")
	fmt.Println("   Prinsip: FQP-9 (Gate Reversibility), GOL B/F")
	fmt.Println()

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "scanner" ||
				base == "_sgvp" || base == "docs" || base == "node_modules" {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanPathSafety(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("❌ Walk error:", err)
		return
	}

	fmt.Printf("\n[🛤️] Selesai! Findings: %d\n", len(pathFindings))
	for _, f := range pathFindings {
		fmt.Printf("🚨 [%s] %s | %s:%d (func %s)\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_path_safety_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writePathReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanPathSafety(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// ─── Check 1: Import "path" instead of "path/filepath" ───
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if importPath == "path" {
			pathFindings = append(pathFindings, PathFinding{
				Level: "HIGH",
				Type:  "Non-OS-Aware Path Import",
				File:  filePath,
				Line:  fset.Position(imp.Pos()).Line,
				Func:  "(import)",
				Message: `Import "path" bukan "path/filepath". Package "path" hanya untuk URL/slash paths, ` +
					`TIDAK menangani backslash Windows. Ganti dengan "path/filepath" untuk cross-OS safety.`,
			})
		}
	}

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcName := fn.Name.Name

		// ─── Check 2: Hardcoded path separators ───
		checkHardcodedSeparators(fset, fn, filePath, funcName)

		// ─── Check 3: File ops without SafeJoin ───
		checkFileOpsWithoutSafeJoin(fset, fn, filePath, funcName)

		// ─── Check 4: Hardcoded OS-specific paths ───
		checkHardcodedOSPaths(fset, fn, filePath, funcName)

		// ─── Check 5: Missing filepath.Clean before security ops ───
		checkMissingClean(fset, fn, filePath, funcName)

		// ─── Check 6: NTFS ADS / Reserved name detection ───
		checkNTFSProtection(fset, fn, filePath, funcName)

		// ─── Check 7: Symlink TOCTOU ───
		checkSymlinkTOCTOU(fset, fn, filePath, funcName)
	}
}

// Check 2: Hardcoded "/" or "\\" separators in filepath ops
func checkHardcodedSeparators(fset *token.FileSet, fn *ast.FuncDecl, filePath, funcName string) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Look for string concatenation with "/" in file path contexts
		// Specifically: someVar + "/" + otherVar  or  someVar + "\\" + otherVar
		binExpr, ok := n.(*ast.BinaryExpr)
		if ok && binExpr.Op.String() == "+" {
			if lit, ok := binExpr.Y.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				val := strings.Trim(lit.Value, `"`)
				if val == "/" || val == "\\" || strings.HasPrefix(val, "/") {
					// Check if this is inside a file operation context
					pathFindings = append(pathFindings, PathFinding{
						Level: "MEDIUM",
						Type:  "Hardcoded Path Separator",
						File:  filePath,
						Line:  fset.Position(lit.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							`String "%s" digunakan sebagai path separator. `+
								`Di Windows, separator adalah "\\", di Unix adalah "/". `+
								`Gunakan filepath.Join() untuk cross-OS compatibility.`,
							val),
					})
				}
			}
		}

		// Check for strings.Replace(path, "/", "\\", -1) or vice versa — OS-specific hack
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isIdent(sel.X, "strings") && sel.Sel.Name == "Replace" && len(call.Args) >= 3 {
				arg1 := extractStringLit(call.Args[1])
				arg2 := extractStringLit(call.Args[2])
				if (arg1 == "/" && arg2 == "\\") || (arg1 == "\\" && arg2 == "/") {
					pathFindings = append(pathFindings, PathFinding{
						Level: "MEDIUM",
						Type:  "Manual Separator Conversion",
						File:  filePath,
						Line:  fset.Position(call.Pos()).Line,
						Func:  funcName,
						Message: `strings.Replace untuk konversi "/" <-> "\\" adalah anti-pattern. ` +
							`Gunakan filepath.FromSlash() atau filepath.ToSlash() yang sudah ` +
							`OS-aware dan menangani edge cases.`,
					})
				}
			}
		}

		return true
	})
}

// Check 3: File operations (Open, ReadFile, WriteFile, Create, Remove) without SafeJoin
func checkFileOpsWithoutSafeJoin(fset *token.FileSet, fn *ast.FuncDecl, filePath, funcName string) {
	// Track if function uses SafeJoin or filepath.Rel validation
	hasSafeJoin := false
	hasRelValidation := false

	type fileOp struct {
		name string
		line int
	}
	var unsafeOps []fileOp

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Detect SafeJoin or equivalent validation
		if sel.Sel.Name == "SafeJoin" || sel.Sel.Name == "EvalSymlinks" {
			hasSafeJoin = true
		}
		if sel.Sel.Name == "Rel" && isIdent(sel.X, "filepath") {
			hasRelValidation = true
		}

		// Detect file operations
		if isIdent(sel.X, "os") {
			dangerousOps := map[string]bool{
				"Open": true, "OpenFile": true, "Create": true,
				"ReadFile": true, "WriteFile": true,
				"Remove": true, "RemoveAll": true,
				"MkdirAll": true, "Mkdir": true,
				"Stat": true, "Lstat": true,
			}
			if dangerousOps[sel.Sel.Name] {
				// Check if path argument contains user-controllable input
				// Heuristic: if path comes from a function parameter (not a constant)
				if len(call.Args) > 0 {
					if !isStringLiteral(call.Args[0]) {
						unsafeOps = append(unsafeOps, fileOp{
							name: "os." + sel.Sel.Name,
							line: fset.Position(call.Pos()).Line,
						})
					}
				}
			}
		}

		return true
	})

	// Only flag exported functions that do file ops without SafeJoin
	// (exported functions are reachable from external input)
	if len(unsafeOps) > 0 && !hasSafeJoin && !hasRelValidation && ast.IsExported(funcName) {
		// Limit to first 2 findings per function to reduce noise
		limit := 2
		if limit > len(unsafeOps) {
			limit = len(unsafeOps)
		}
		for _, op := range unsafeOps[:limit] {
			pathFindings = append(pathFindings, PathFinding{
				Level: "HIGH",
				Type:  "File Op Without Path Validation",
				File:  filePath,
				Line:  op.line,
				Func:  funcName,
				Message: fmt.Sprintf(
					`%s() dipanggil di exported function tanpa SafeJoin/filepath.Rel validation. `+
						`Path traversal attack (../../etc/passwd) bisa bypass workspace boundary. `+
						`Validasi path dengan SafeJoin() atau filepath.Rel() sebelum operasi file.`,
					op.name),
			})
		}
	}
}

// Check 4: Hardcoded OS-specific paths
func checkHardcodedOSPaths(fset *token.FileSet, fn *ast.FuncDecl, filePath, funcName string) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		val := strings.Trim(lit.Value, `"`)
		valLow := strings.ToLower(val)

		// Unix-specific paths
		unixPaths := []string{"/tmp/", "/etc/", "/dev/", "/proc/", "/var/", "/usr/"}
		for _, p := range unixPaths {
			if strings.HasPrefix(valLow, p) {
				pathFindings = append(pathFindings, PathFinding{
					Level: "MEDIUM",
					Type:  "Hardcoded Unix Path",
					File:  filePath,
					Line:  fset.Position(lit.Pos()).Line,
					Func:  funcName,
					Message: fmt.Sprintf(
						`Path "%s" hardcoded — tidak ada di Windows. `+
							`Gunakan os.TempDir(), os.UserConfigDir(), atau filepath.Join(os.Getenv("...")) `+
							`untuk cross-OS portability.`, val),
				})
				return true
			}
		}

		// Windows-specific paths
		if len(val) >= 3 && val[1] == ':' && (val[2] == '\\' || val[2] == '/') {
			pathFindings = append(pathFindings, PathFinding{
				Level: "MEDIUM",
				Type:  "Hardcoded Windows Drive Path",
				File:  filePath,
				Line:  fset.Position(lit.Pos()).Line,
				Func:  funcName,
				Message: fmt.Sprintf(
					`Path "%s" hardcoded — tidak portabel ke Linux/macOS. `+
						`Gunakan os.TempDir(), os.UserHomeDir(), atau os.UserConfigDir() `+
						`untuk cross-OS portability.`, val),
			})
		}

		return true
	})
}

// Check 5: File path used in security decision without filepath.Clean
func checkMissingClean(fset *token.FileSet, fn *ast.FuncDecl, filePath, funcName string) {
	hasClean := false
	hasAbs := false
	hasContains := false
	var containsLine int

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Track Clean/Abs calls
		if isIdent(sel.X, "filepath") {
			if sel.Sel.Name == "Clean" || sel.Sel.Name == "Abs" || sel.Sel.Name == "EvalSymlinks" {
				hasClean = true
			}
			if sel.Sel.Name == "Abs" {
				hasAbs = true
			}
		}

		// Detect path-based security decisions: strings.Contains/HasPrefix on path
		if isIdent(sel.X, "strings") {
			if sel.Sel.Name == "Contains" || sel.Sel.Name == "HasPrefix" ||
				sel.Sel.Name == "HasSuffix" {
				// Check if args contain path-like patterns
				for _, arg := range call.Args {
					if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						val := strings.Trim(lit.Value, `"`)
						if strings.Contains(val, "..") || strings.Contains(val, "/") ||
							strings.Contains(val, "\\") || strings.HasPrefix(val, ".") {
							hasContains = true
							containsLine = fset.Position(call.Pos()).Line
						}
					}
				}
			}
		}

		return true
	})

	// Flag: path security check without filepath.Clean/Abs
	if hasContains && !hasClean && !hasAbs {
		pathFindings = append(pathFindings, PathFinding{
			Level: "HIGH",
			Type:  "Path Security Check Without Clean/Abs",
			File:  filePath,
			Line:  containsLine,
			Func:  funcName,
			Message: `strings.Contains/HasPrefix digunakan untuk validasi path tanpa filepath.Clean() ` +
				`atau filepath.Abs() terlebih dahulu. Path "foo/../../../etc/passwd" bisa bypass check ` +
				`yang hanya memeriksa prefix. Selalu Clean/Abs dulu sebelum compare.`,
		})
	}
}

// Check 6: NTFS Alternate Data Stream & Reserved Name detection
func checkNTFSProtection(fset *token.FileSet, fn *ast.FuncDecl, filePath, funcName string) {
	// Look for filename validation that checks for ":" or Windows reserved names
	hasColonCheck := false
	hasReservedCheck := false
	hasFileOp := false
	var fileOpLine int

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		// Check for string checks on ":"
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			val := strings.Trim(lit.Value, `"`)
			if val == ":" || strings.Contains(val, "ADS") || strings.Contains(val, "alternate") {
				hasColonCheck = true
			}
			// Check for reserved name awareness
			valUp := strings.ToUpper(val)
			if valUp == "CON" || valUp == "PRN" || valUp == "NUL" || valUp == "AUX" ||
				valUp == "COM1" || valUp == "LPT1" {
				hasReservedCheck = true
			}
		}

		// Detect file creation ops (Create, WriteFile, OpenFile)
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if isIdent(sel.X, "os") {
					if sel.Sel.Name == "Create" || sel.Sel.Name == "OpenFile" ||
						sel.Sel.Name == "WriteFile" || sel.Sel.Name == "MkdirAll" {
						hasFileOp = true
						fileOpLine = fset.Position(call.Pos()).Line
					}
				}
			}
		}
		return true
	})

	// Only flag functions that create files AND take user-controlled filenames
	// AND don't have NTFS protection
	if hasFileOp && !hasColonCheck && !hasReservedCheck && ast.IsExported(funcName) {
		// Check if function has a string parameter that could be a filename
		if fn.Type.Params != nil {
			for _, param := range fn.Type.Params.List {
				if id, ok := param.Type.(*ast.Ident); ok && id.Name == "string" {
					for _, name := range param.Names {
						nameLow := strings.ToLower(name.Name)
						if strings.Contains(nameLow, "path") || strings.Contains(nameLow, "file") ||
							strings.Contains(nameLow, "name") || strings.Contains(nameLow, "dir") {
							pathFindings = append(pathFindings, PathFinding{
								Level: "MEDIUM",
								Type:  "Missing NTFS ADS/Reserved Name Check",
								File:  filePath,
								Line:  fileOpLine,
								Func:  funcName,
								Message: fmt.Sprintf(
									`Exported function menerima parameter "%s" (string) dan melakukan file create `+
										`tanpa validasi NTFS ADS (colon ":") atau reserved names (CON, PRN, NUL). `+
										`Di Windows: "file.txt:hidden" creates ADS stream, "CON" causes hang. `+
										`Tambahkan sanitasi filename untuk cross-OS safety.`,
									name.Name),
							})
							return // One finding per function is enough
						}
					}
				}
			}
		}
	}
}

// Check 7: Symlink TOCTOU — os.Stat without os.Lstat/EvalSymlinks
func checkSymlinkTOCTOU(fset *token.FileSet, fn *ast.FuncDecl, filePath, funcName string) {
	hasStat := false
	hasLstat := false
	hasEvalSymlinks := false
	var statLine int

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if isIdent(sel.X, "os") {
			if sel.Sel.Name == "Stat" {
				hasStat = true
				statLine = fset.Position(call.Pos()).Line
			}
			if sel.Sel.Name == "Lstat" {
				hasLstat = true
			}
		}
		if isIdent(sel.X, "filepath") && sel.Sel.Name == "EvalSymlinks" {
			hasEvalSymlinks = true
		}
		return true
	})

	// Only flag if function uses os.Stat for security decisions
	// (e.g., checking if file exists before writing) without symlink awareness
	nameLow := strings.ToLower(funcName)
	isSecurityFunc := strings.Contains(nameLow, "check") || strings.Contains(nameLow, "valid") ||
		strings.Contains(nameLow, "safe") || strings.Contains(nameLow, "allow") ||
		strings.Contains(nameLow, "intercept") || strings.Contains(nameLow, "guard") ||
		strings.Contains(nameLow, "sensitive") || strings.Contains(nameLow, "perm")

	if hasStat && !hasLstat && !hasEvalSymlinks && isSecurityFunc {
		pathFindings = append(pathFindings, PathFinding{
			Level: "HIGH",
			Type:  "Symlink TOCTOU in Security Check",
			File:  filePath,
			Line:  statLine,
			Func:  funcName,
			Message: `os.Stat() digunakan di security function tanpa os.Lstat() atau filepath.EvalSymlinks(). ` +
				`os.Stat follows symlinks — attacker bisa membuat symlink ke file sensitif SETELAH check ` +
				`tapi SEBELUM operasi (TOCTOU). Gunakan os.Lstat() untuk cek symlink, ` +
				`atau filepath.EvalSymlinks() untuk resolve real path.`,
		})
	}
}

// ─── Helpers ───────────────────────────────────────────────

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isStringLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

func extractStringLit(e ast.Expr) string {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		return strings.Trim(lit.Value, `"`)
	}
	return ""
}

// ─── Report Writer ─────────────────────────────────────────

func writePathReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 🛤️ EXT Path Safety Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_path_safety_scanner v1\n")
	out.WriteString("> **Focus:** Cross-OS portability (Windows, Linux, macOS/Darwin)\n")
	out.WriteString("> **Prinsip:** FQP-9 (Gate Reversibility), GOL §B (Protected Core), GOL §F (Gerbang)\n\n")

	out.WriteString("## Checks Performed\n\n")
	out.WriteString("| # | Check | Level | Keterangan |\n")
	out.WriteString("|---|-------|-------|------------|\n")
	out.WriteString("| 1 | Non-OS-Aware Import | HIGH | `import \"path\"` bukan `\"path/filepath\"` |\n")
	out.WriteString("| 2 | Hardcoded Separator | MEDIUM | `\"/\"` atau `\"\\\\\"` manual |\n")
	out.WriteString("| 3 | File Op Without SafeJoin | HIGH | Path traversal risk |\n")
	out.WriteString("| 4 | Hardcoded OS Path | MEDIUM | `/tmp/`, `C:\\`, `/etc/` |\n")
	out.WriteString("| 5 | Missing filepath.Clean | HIGH | Security check tanpa normalize |\n")
	out.WriteString("| 6 | NTFS ADS/Reserved Names | MEDIUM | `file:hidden`, `CON`, `NUL` |\n")
	out.WriteString("| 7 | Symlink TOCTOU | HIGH | os.Stat di security func |\n\n")

	if len(pathFindings) == 0 {
		out.WriteString("✅ *Tidak ditemukan masalah path safety.*\n")
		return
	}

	// Count by level
	crit, high, med, low := 0, 0, 0, 0
	for _, f := range pathFindings {
		switch f.Level {
		case "CRITICAL":
			crit++
		case "HIGH":
			high++
		case "MEDIUM":
			med++
		case "LOW":
			low++
		}
	}

	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 Critical: %d | 🟠 High: %d | 🟡 Medium: %d | 🔵 Low: %d)\n\n",
		len(pathFindings), crit, high, med, low))

	// Count by type
	typeCounts := make(map[string]int)
	for _, f := range pathFindings {
		typeCounts[f.Type]++
	}
	out.WriteString("### Ringkasan per Tipe\n\n")
	out.WriteString("| Tipe | Jumlah |\n")
	out.WriteString("|------|--------|\n")
	for t, c := range typeCounts {
		out.WriteString(fmt.Sprintf("| %s | %d |\n", t, c))
	}
	out.WriteString("\n")

	for i, f := range pathFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}
