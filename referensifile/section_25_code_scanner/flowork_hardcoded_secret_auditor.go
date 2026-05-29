//go:build ignore

// ext_hardcoded_secret_scanner — mendeteksi API key, password, token,
// dan credential yang di-hardcode di source code.
//
// Penyebab #1 data breach di dunia nyata. Scanner ini memeriksa:
//  1. Pattern API key: sk-, ghp_, gho_, AKIA, xoxb-, rk-
//  2. Hardcoded password/token assignment
//  3. PEM private key blocks
//  4. Long hex/base64 strings yang kemungkinan secret
//  5. Bearer/Basic auth literals
//
// Prinsip: GOL §C (Sensitive Config), FQP-4 (SGVP Guard)
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

type SecretFinding struct {
	Level   string
	Type    string
	File    string
	Line    int
	Func    string
	Message string
}

var secretFindings []SecretFinding

// Known API key prefixes
var apiKeyPrefixes = []struct {
	prefix  string
	service string
}{
	{"sk-", "OpenAI/Stripe"},
	{"sk_live_", "Stripe Live"},
	{"sk_test_", "Stripe Test"},
	{"ghp_", "GitHub PAT"},
	{"gho_", "GitHub OAuth"},
	{"ghs_", "GitHub App"},
	{"github_pat_", "GitHub Fine-grained PAT"},
	{"AKIA", "AWS Access Key"},
	{"xoxb-", "Slack Bot Token"},
	{"xoxp-", "Slack User Token"},
	{"xapp-", "Slack App Token"},
	{"rk-", "OpenRouter Key"},
	{"glpat-", "GitLab PAT"},
	{"AIza", "Google API Key"},
	{"ya29.", "Google OAuth Token"},
	{"whsec_", "Webhook Secret"},
	{"SG.", "SendGrid Key"},
	{"key-", "Mailgun Key"},
}

// Regex for long hex strings (likely tokens)
var hexTokenRe = regexp.MustCompile(`^[0-9a-fA-F]{32,}$`)

// Regex for base64-ish secrets (40+ chars, mixed case, digits)
var base64SecretRe = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{40,}$`)

// Sensitive variable name patterns
var sensitiveVarNames = []string{
	"password", "passwd", "secret", "apikey", "api_key",
	"token", "auth", "credential", "private_key", "privatekey",
	"access_key", "accesskey", "secret_key", "secretkey",
}

func main() {
	fmt.Println("🔐 [EXT_HARDCODED_SECRET v1] Scanning for hardcoded secrets...")
	fmt.Println("   Prinsip: GOL C (Sensitive Config), FQP-4 (SGVP Guard)")
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
			scanSecrets(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("Walk error:", err)
		return
	}

	fmt.Printf("\n[🔐] Selesai! Findings: %d\n", len(secretFindings))
	for _, f := range secretFindings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_hardcoded_secret_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeSecretReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanSecrets(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcName := fn.Name.Name

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// Check assignments: varName := "literal"
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}

			for i, rhs := range assign.Rhs {
				lit, ok := rhs.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val := strings.Trim(lit.Value, `"`+"`")
				if len(val) < 8 {
					continue
				}

				// Get variable name
				varName := ""
				if i < len(assign.Lhs) {
					if id, ok := assign.Lhs[i].(*ast.Ident); ok {
						varName = id.Name
					}
				}
				varLow := strings.ToLower(varName)

				// Check 1: API key prefix match
				for _, kp := range apiKeyPrefixes {
					if strings.HasPrefix(val, kp.prefix) && len(val) > len(kp.prefix)+8 {
						secretFindings = append(secretFindings, SecretFinding{
							Level: "CRITICAL",
							Type:  "Hardcoded API Key",
							File:  filePath,
							Line:  fset.Position(lit.Pos()).Line,
							Func:  funcName,
							Message: fmt.Sprintf(
								"Kemungkinan %s API key hardcoded: %q = %q (prefix %s). "+
									"JANGAN hardcode secret di source. Gunakan os.Getenv() atau config file terenkripsi.",
								kp.service, varName, truncate(val, 16), kp.prefix),
						})
						return true
					}
				}

				// Check 2: PEM private key block
				if strings.Contains(val, "-----BEGIN") && strings.Contains(val, "PRIVATE") {
					secretFindings = append(secretFindings, SecretFinding{
						Level: "CRITICAL",
						Type:  "Hardcoded Private Key",
						File:  filePath,
						Line:  fset.Position(lit.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							"PEM private key hardcoded di variable %q. "+
								"Private key HARUS di-load dari file atau vault, BUKAN di-embed di source.",
							varName),
					})
					return true
				}

				// Check 3: Sensitive variable name + literal value
				for _, sv := range sensitiveVarNames {
					if strings.Contains(varLow, sv) {
						// Exclude common false positives
						if val == "" || strings.HasPrefix(val, "FLOWORK_") ||
							strings.HasPrefix(val, "http") || strings.HasPrefix(val, "$") ||
							val == "Bearer " || val == "Basic " || val == "token" ||
							strings.Contains(val, "%s") || strings.Contains(val, "env:") {
							break
						}
						secretFindings = append(secretFindings, SecretFinding{
							Level: "HIGH",
							Type:  "Suspicious Secret Assignment",
							File:  filePath,
							Line:  fset.Position(lit.Pos()).Line,
							Func:  funcName,
							Message: fmt.Sprintf(
								"Variable %q (mengandung %q) di-assign literal string %q. "+
									"Jika ini secret, gunakan os.Getenv() atau config terenkripsi.",
								varName, sv, truncate(val, 20)),
						})
						return true
					}
				}

				// Check 4: Long hex token
				if hexTokenRe.MatchString(val) && len(val) >= 32 {
					secretFindings = append(secretFindings, SecretFinding{
						Level: "MEDIUM",
						Type:  "Possible Hardcoded Token (Hex)",
						File:  filePath,
						Line:  fset.Position(lit.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							"String hex panjang (%d chars) di-assign ke %q: %q. "+
								"Kemungkinan hardcoded token/hash. Verifikasi ini bukan secret.",
							len(val), varName, truncate(val, 16)),
					})
				}

				// Check 5: Bearer/Basic auth with actual token
				if strings.HasPrefix(val, "Bearer ") && len(val) > 15 {
					secretFindings = append(secretFindings, SecretFinding{
						Level: "CRITICAL",
						Type:  "Hardcoded Bearer Token",
						File:  filePath,
						Line:  fset.Position(lit.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							"Bearer token hardcoded: %q. "+
								"Auth header HARUS dibuild dari os.Getenv(), bukan literal.",
							truncate(val, 20)),
					})
				}
			}
			return true
		})
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func writeSecretReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 🔐 EXT Hardcoded Secret Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_hardcoded_secret_scanner v1\n")
	out.WriteString("> **Prinsip:** GOL C (Sensitive Config), FQP-4 (SGVP Guard)\n")
	out.WriteString("> **Target:** API keys, passwords, tokens, PEM keys hardcoded di source\n\n")

	if len(secretFindings) == 0 {
		out.WriteString("✅ *Tidak ditemukan hardcoded secret.*\n")
		return
	}

	crit, high, med := 0, 0, 0
	for _, f := range secretFindings {
		switch f.Level {
		case "CRITICAL":
			crit++
		case "HIGH":
			high++
		case "MEDIUM":
			med++
		}
	}
	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 Critical: %d | 🟠 High: %d | 🟡 Medium: %d)\n\n",
		len(secretFindings), crit, high, med))

	for i, f := range secretFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}
