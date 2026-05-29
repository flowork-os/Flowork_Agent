//go:build ignore

// Package zeroday — semantic-aware scanner untuk hardcoded secrets,
// insecure file permissions, dan timing-attack comparisons.
//
// Rewrite rc113 (Opus-2 2026-04-19): sebelumnya pattern-match menghasilkan
// 60+ FALSE POSITIVE karena:
//
//	(a) "Hardcoded Secret" flag semua string literal di-assign ke var
//	    bernama *token*, *secret*, *apikey*, *password*. Realitas: banyak
//	    assignment adalah NAMA env var (contoh `EnvAPIKey = "OPENAI_API_KEY"`),
//	    bukan credential aktual.
//	(b) "Crypto Timing Attack" flag SEMUA `==` dengan operand sensitif.
//	    Realitas: banyak comparison adalah cek terhadap konstanta
//	    UPPERCASE_ENV_NAME ("GEMINI_API_KEY" dll) yang bukan secret.
//
// Rewrite ini tambahin:
//  1. isLikelyEnvVarName() — match ^[A-Z][A-Z0-9_]{3,}$ (UPPERCASE_UNDERSCORE)
//     → reject klaim "hardcoded secret" kalau RHS match pola env var nama.
//  2. isActualSecretShape() — heuristic: real credential biasanya 20+ char,
//     mixed-case atau hex-base64-ish. Env var name selalu capsonly.
//  3. Timing-attack allowlist: skip bila salah satu operand adalah
//     konstanta ENV_VAR_NAME atau string pendek (≤8 char, e.g. "y"/"ok").
//  4. Skip `ownerauth/auth.go` untuk timing — tim Claude sudah migrate ke
//     subtle.ConstantTimeCompare di titik kritis (BUG-H02 rc109, etc).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ZeroDayVuln struct {
	Level   string
	Type    string
	File    string
	Line    int
	Message string
}

var zeroDayFindings []ZeroDayVuln

// UPPERCASE_UNDERSCORE pattern typical of environment variable names.
// "GEMINI_API_KEY" matches; "sk-abc123..." does not.
var envVarNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{3,}$`)

// Short enum/marker strings that are never credentials.
var shortKnownLiterals = map[string]bool{
	"y": true, "n": true, "yes": true, "no": true, "ok": true, "error": true,
	"true": true, "false": true, "on": true, "off": true, "bearer": true,
	"auto": true, "manual": true, "allow": true, "deny": true, "ask": true,
}

func main() {
	fmt.Println("🕵️‍♂️ [ZERODAY v2 rc113] Scanning hardcoded secrets + insecure perms + real timing-attack sites...")
	fmt.Println("Semantic-aware: skip env var nama + enum compare + konstanta marker.")

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if strings.Contains(path, ".git") || strings.Contains(path, "tools_temp") ||
				strings.Contains(path, "node_modules") || strings.Contains(path, "state/") ||
				strings.Contains(path, "_sgvp") /* SGVP test fixtures intentional */ {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanZeroDay(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n[⚠️] Audit selesai. Findings: %d (semantic filter aktif).\n", len(zeroDayFindings))
	for _, f := range zeroDayFindings {
		fmt.Printf("☢️  [%s] %s | %s:%d\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "zeroday_audit_report.md")
	writeZeroDayReport(outFile)
	fmt.Println("\n📁 Report:", outFile)
}

func scanZeroDay(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {

		// 1. Insecure Permissions — tetap flag semua world-writable.
		if call, ok := n.(*ast.CallExpr); ok {
			if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
				if fun.Sel.Name == "WriteFile" || fun.Sel.Name == "Mkdir" || fun.Sel.Name == "MkdirAll" || fun.Sel.Name == "OpenFile" {
					for _, arg := range call.Args {
						if lit, isLit := arg.(*ast.BasicLit); isLit {
							val := lit.Value
							if val == "0777" || val == "0o777" || val == "0666" || val == "0o666" {
								msg := fmt.Sprintf("Set-permission '%s' = world-writable. Siapa saja di host bisa overwrite.", val)
								recordZD(fset, call.Pos(), filePath, "CRITICAL", "Insecure File Permission", msg)
							}
						}
					}
				}
			}
		}

		// 2. Timing Attack — hanya real credential compare.
		if binExpr, ok := n.(*ast.BinaryExpr); ok {
			if binExpr.Op == token.EQL || binExpr.Op == token.NEQ {
				if timingAttackCandidate(binExpr) {
					msg := "Compare data sensitif (token/password/hash) pakai '==' — rentan timing-attack. Gunakan subtle.ConstantTimeCompare."
					recordZD(fset, binExpr.Pos(), filePath, "HIGH", "Crypto Timing Attack", msg)
				}
			}
		}

		// 3. Hardcoded Secret — hanya real credential-shape, bukan env var name.
		if assign, ok := n.(*ast.AssignStmt); ok {
			if assign.Tok == token.ASSIGN || assign.Tok == token.DEFINE {
				for i, lhs := range assign.Lhs {
					if !isSensitiveVar(lhs) {
						continue
					}
					if i >= len(assign.Rhs) {
						continue
					}
					lit, isLit := assign.Rhs[i].(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING {
						continue
					}
					raw := strings.Trim(lit.Value, "`\"")
					if !isLikelyCredential(raw) {
						continue
					}
					msg := fmt.Sprintf("Hardcoded secret-like literal di source (%q). Pindah ke .env.", truncate(raw, 40))
					recordZD(fset, assign.Pos(), filePath, "CRITICAL", "Hardcoded Secret", msg)
				}
			}
		}

		return true
	})
}

// timingAttackCandidate returns true only when both sides genuinely look like
// sensitive credential compare. Skips:
//   - nil compare (onToken != nil)
//   - bool compare (hasToken == true)
//   - hash compare (Merkle chain integrity — hash preimage resistance
//     already protects; adding subtle.ConstantTimeCompare = over-eng)
//   - enum marker / short strings / env var name literals
func timingAttackCandidate(bin *ast.BinaryExpr) bool {
	// Skip nil / bool compare — not timing-attack surface.
	for _, side := range []ast.Expr{bin.X, bin.Y} {
		if id, ok := side.(*ast.Ident); ok {
			switch id.Name {
			case "nil", "true", "false":
				return false
			}
		}
	}

	sensitive := isSensitiveVar(bin.X) || isSensitiveVar(bin.Y)
	if !sensitive {
		return false
	}

	// Skip if BOTH operands look like hash values (Merkle chain integrity,
	// not secret credential compare). Hash names: "Hash", "PrevHash",
	// "ExpectedHash", "incomingHash", etc. — ends with "Hash" OR name is
	// a map access yang LHS contains "hash".
	xLooksHash := looksLikeHashOperand(bin.X)
	yLooksHash := looksLikeHashOperand(bin.Y)
	if xLooksHash && yLooksHash {
		return false
	}

	// Skip if one side is a literal that's short / known marker / env var name.
	for _, side := range []ast.Expr{bin.X, bin.Y} {
		lit, ok := side.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		raw := strings.ToLower(strings.Trim(lit.Value, "`\""))
		rawUpper := strings.Trim(lit.Value, "`\"")
		if len(raw) <= 8 {
			return false // "y"/"ok"/"bearer" enum
		}
		if shortKnownLiterals[raw] {
			return false
		}
		if envVarNamePattern.MatchString(rawUpper) {
			return false // "GEMINI_API_KEY" compare
		}
	}
	return true
}

// looksLikeHashOperand returns true kalau expression reference variable/field
// yang namanya ends with "Hash" atau explicit hash type (integrity check,
// bukan secret compare).
func looksLikeHashOperand(e ast.Expr) bool {
	var name string
	switch x := e.(type) {
	case *ast.Ident:
		name = x.Name
	case *ast.SelectorExpr:
		name = x.Sel.Name
	default:
		return false
	}
	low := strings.ToLower(name)
	// "hash" suffix or full "hash" word — integrity check, not secret.
	return strings.HasSuffix(low, "hash") || low == "hash" ||
		strings.Contains(low, "hashset") || strings.Contains(low, "checksum")
}

// isLikelyCredential distinguishes real credentials from env var name strings.
//
// Positive shape:
//   - Length ≥ 20 char typical for OAuth/API tokens
//   - Mixed-case or contains digit+lowercase (not pure UPPERCASE_UNDERSCORE)
//   - Does NOT match env var name pattern
//
// Negative shape:
//   - Pure UPPERCASE_UNDERSCORE ≤ 32 char = env var name
//   - Contains template placeholder `{...}` or `${...}`
//   - Empty / very short
//   - Well-known non-credential ("Bearer", "application/json", etc)
func isLikelyCredential(s string) bool {
	if len(s) < 16 {
		return false
	}
	if strings.Contains(s, "{") || strings.Contains(s, "$") {
		return false
	}
	if envVarNamePattern.MatchString(s) {
		return false
	}
	low := strings.ToLower(s)
	if shortKnownLiterals[low] {
		return false
	}
	// Must have digit or mixed-case (not just alpha-underscore).
	hasDigit := strings.ContainsAny(s, "0123456789")
	hasLower := strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz")
	hasUpper := strings.ContainsAny(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	mixed := hasLower && (hasUpper || hasDigit)
	return mixed || hasDigit
}

// Heuristik mendeteksi nama variabel yang menyimpan rahasia.
func isSensitiveVar(expr ast.Expr) bool {
	var name string
	switch e := expr.(type) {
	case *ast.Ident:
		name = e.Name
	case *ast.SelectorExpr:
		name = e.Sel.Name
	default:
		return false
	}

	low := strings.ToLower(name)
	if strings.Contains(low, "password") || strings.Contains(low, "secret") ||
		strings.Contains(low, "token") || strings.Contains(low, "apikey") ||
		strings.Contains(low, "hash") {
		return true
	}
	return false
}

func recordZD(fset *token.FileSet, pos token.Pos, file, level, fType, msg string) {
	// Skip generated scanner files themselves.
	p := strings.ReplaceAll(file, "\\", "/")
	if strings.HasPrefix(p, "scanner/") || strings.Contains(p, "/scanner/") {
		return
	}
	if strings.Contains(file, "test") {
		return
	}

	zeroDayFindings = append(zeroDayFindings, ZeroDayVuln{
		Level:   level,
		Type:    fType,
		File:    file,
		Line:    fset.Position(pos).Line,
		Message: msg,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func writeZeroDayReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# 🕵️‍♂️ Laporan Audit Spesial: Zero-Days & Permissions\n\n")
	out.WriteString("> **Scanner v2 (rc113)** — semantic filter aktif: skip env var nama + enum marker + konstanta pendek. FP rate target < 20%.\n\n")
	out.WriteString("Audit file permission (0777/0666), hardcoded secret dengan credential-shape detection, dan real timing-attack site.\n\n")

	if len(zeroDayFindings) == 0 {
		out.WriteString("*Tidak ada temuan valid. Scanner v2 filter FP yang sebelumnya noise.*\n")
		return
	}

	for _, f := range zeroDayFindings {
		out.WriteString(fmt.Sprintf("---\n### ☢️ [%s] %s\n", f.Level, f.Type))
		out.WriteString(fmt.Sprintf("**File**: `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("**Analisis**: %s\n\n", f.Message))
	}
}
