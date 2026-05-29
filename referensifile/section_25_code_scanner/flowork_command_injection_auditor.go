//go:build ignore

// ext_command_injection_scanner — mendeteksi command injection vectors.
//
// Scanner ini memeriksa:
//  1. exec.Command dengan argument dari variabel (bukan literal)
//  2. exec.Command("sh", "-c", userInput) — classic injection
//  3. exec.Command("bash", "-c", ...) tanpa sandbox
//  4. fmt.Sprintf digunakan untuk menyusun command string
//  5. Missing sandbox.RunArgv wrapper
//
// Prinsip: GOL F (Gerbang Pertahanan), FQP-4 (SGVP Guard)
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

type CmdInjFinding struct {
	Level   string
	Type    string
	File    string
	Line    int
	Func    string
	Message string
}

var cmdFindings []CmdInjFinding

func main() {
	fmt.Println("💉 [EXT_CMD_INJECTION v1] Scanning for command injection vectors...")
	fmt.Println("   Prinsip: GOL F (Gerbang Pertahanan), FQP-4 (SGVP Guard)")
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
			scanCmdInjection(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("Walk error:", err)
		return
	}

	fmt.Printf("\n[💉] Selesai! Findings: %d\n", len(cmdFindings))
	for _, f := range cmdFindings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_command_injection_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeCmdReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanCmdInjection(filePath string) {
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

		// Track whether function uses sandbox
		hasSandbox := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "RunArgv" || sel.Sel.Name == "Run" {
						if isIdent(sel.X, "sandbox") {
							hasSandbox = true
						}
					}
				}
			}
			return true
		})

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Detect exec.Command and exec.CommandContext
			isExecCmd := false
			if isIdent(sel.X, "exec") &&
				(sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
				isExecCmd = true
			}
			if !isExecCmd {
				return true
			}

			// Get the command argument (first arg for Command, second for CommandContext)
			cmdArgIdx := 0
			if sel.Sel.Name == "CommandContext" {
				cmdArgIdx = 1
			}
			if cmdArgIdx >= len(call.Args) {
				return true
			}

			cmdArg := call.Args[cmdArgIdx]
			cmdLit := extractStringLit(cmdArg)

			// Check 1: Shell invocation with "-c" — classic injection
			shellCmds := []string{"sh", "bash", "zsh", "cmd", "cmd.exe", "powershell", "powershell.exe"}
			for _, sh := range shellCmds {
				if cmdLit == sh {
					// Check if next arg is "-c" or "/c"
					nextIdx := cmdArgIdx + 1
					if nextIdx < len(call.Args) {
						nextLit := extractStringLit(call.Args[nextIdx])
						if nextLit == "-c" || nextLit == "/c" || nextLit == "/C" || nextLit == "-Command" {
							// Check if the command string (3rd arg) is dynamic
							cmdStrIdx := nextIdx + 1
							if cmdStrIdx < len(call.Args) {
								if !isStringLiteral(call.Args[cmdStrIdx]) {
									level := "HIGH"
									if !hasSandbox {
										level = "CRITICAL"
									}
									cmdFindings = append(cmdFindings, CmdInjFinding{
										Level: level,
										Type:  "Shell Injection Vector",
										File:  filePath,
										Line:  fset.Position(call.Pos()).Line,
										Func:  funcName,
										Message: fmt.Sprintf(
											`exec.%s("%s", "%s", <dynamic>) — command string dinamis. `+
												`Jika input berasal dari user/AI/network, attacker bisa inject: `+
												`"valid; rm -rf /". Gunakan exec.Command(binary, arg1, arg2...) `+
												`TANPA shell, atau sandbox.RunArgv().`,
											sel.Sel.Name, sh, nextLit),
									})
								}
							}
						}
					}
				}
			}

			// Check 2: exec.Command with fully dynamic command name
			if !isStringLiteral(cmdArg) && !hasSandbox {
				// Skip if command comes from exec.LookPath or filepath.Join (safer patterns)
				if !isLookPathResult(cmdArg) {
					cmdFindings = append(cmdFindings, CmdInjFinding{
						Level: "MEDIUM",
						Type:  "Dynamic Command Name",
						File:  filePath,
						Line:  fset.Position(call.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							"exec.%s() dengan command name dinamis (bukan string literal). "+
								"Jika command name berasal dari user input, attacker bisa execute "+
								"arbitrary binary. Validasi command terhadap whitelist.",
							sel.Sel.Name),
					})
				}
			}

			// Check 3: String concatenation/Sprintf in command args
			for argIdx := cmdArgIdx + 1; argIdx < len(call.Args); argIdx++ {
				arg := call.Args[argIdx]
				if isFmtSprintf(arg) {
					cmdFindings = append(cmdFindings, CmdInjFinding{
						Level: "HIGH",
						Type:  "fmt.Sprintf in Command Arg",
						File:  filePath,
						Line:  fset.Position(arg.Pos()).Line,
						Func:  funcName,
						Message: "fmt.Sprintf() digunakan untuk menyusun argument exec.Command. " +
							"Jika format string interpolasi mengandung user input, " +
							"ini bisa menjadi injection vector. Pisahkan ke argumen terpisah.",
					})
					break
				}
			}

			return true
		})
	}
}

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

func isLookPathResult(e ast.Expr) bool {
	// Heuristic: variable named xxxPath or result of LookPath
	if id, ok := e.(*ast.Ident); ok {
		low := strings.ToLower(id.Name)
		return strings.Contains(low, "path") || strings.Contains(low, "bin") ||
			strings.Contains(low, "executable")
	}
	return false
}

func isFmtSprintf(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return isIdent(sel.X, "fmt") && sel.Sel.Name == "Sprintf"
}

func writeCmdReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 💉 EXT Command Injection Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_command_injection_scanner v1\n")
	out.WriteString("> **Prinsip:** GOL F (Gerbang Pertahanan), FQP-4 (SGVP Guard)\n")
	out.WriteString("> **Target:** Shell injection, dynamic commands, fmt.Sprintf in args\n\n")

	if len(cmdFindings) == 0 {
		out.WriteString("✅ *Tidak ditemukan command injection vector.*\n")
		return
	}

	crit, high, med := 0, 0, 0
	for _, f := range cmdFindings {
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
		len(cmdFindings), crit, high, med))

	for i, f := range cmdFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}
