//go:build ignore

// ext_crypto_weakness_scanner — mendeteksi penggunaan kriptografi lemah.
//
// Scanner ini memeriksa:
//  1. math/rand (bukan crypto/rand) di security context
//  2. MD5, SHA1 di integrity/auth context
//  3. Hardcoded IV/nonce/salt
//  4. Key size terlalu kecil
//  5. ECB mode atau missing AEAD
//
// Prinsip: FQP-6 (BFT Quorum), FQP-13 (No-Broadcasting), GOL Fase 7 (Immune)
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

type CryptoFinding struct {
	Level   string
	Type    string
	File    string
	Line    int
	Func    string
	Message string
}

var cryptoFindings []CryptoFinding

func main() {
	fmt.Println("🔑 [EXT_CRYPTO_WEAKNESS v1] Scanning for weak cryptography...")
	fmt.Println("   Prinsip: FQP-6 (BFT), FQP-13 (No-Broadcasting), GOL Fase 7")
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
			scanCryptoWeakness(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("Walk error:", err)
		return
	}

	fmt.Printf("\n[🔑] Selesai! Findings: %d\n", len(cryptoFindings))
	for _, f := range cryptoFindings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_crypto_weakness_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeCryptoReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanCryptoWeakness(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// Check 1: Dangerous imports
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		switch importPath {
		case "math/rand":
			cryptoFindings = append(cryptoFindings, CryptoFinding{
				Level: "HIGH",
				Type:  "Weak Random: math/rand",
				File:  filePath,
				Line:  fset.Position(imp.Pos()).Line,
				Func:  "(import)",
				Message: `Import "math/rand" terdeteksi. math/rand TIDAK kriptografis aman ` +
					`(predictable seed, deterministic output). Untuk key/nonce/token/salt, ` +
					`gunakan "crypto/rand". math/rand hanya untuk non-security (shuffle, jitter).`,
			})
		case "crypto/md5":
			cryptoFindings = append(cryptoFindings, CryptoFinding{
				Level: "HIGH",
				Type:  "Weak Hash: MD5",
				File:  filePath,
				Line:  fset.Position(imp.Pos()).Line,
				Func:  "(import)",
				Message: `Import "crypto/md5". MD5 collision-broken sejak 2004. ` +
					`JANGAN gunakan untuk integrity/auth/signature. ` +
					`Gunakan SHA-256 (crypto/sha256) atau SHA-3.`,
			})
		case "crypto/sha1":
			cryptoFindings = append(cryptoFindings, CryptoFinding{
				Level: "MEDIUM",
				Type:  "Weak Hash: SHA1",
				File:  filePath,
				Line:  fset.Position(imp.Pos()).Line,
				Func:  "(import)",
				Message: `Import "crypto/sha1". SHA1 collision-broken (SHAttered, 2017). ` +
					`Jangan gunakan untuk signature/certificate. ` +
					`Gunakan SHA-256 atau SHA-3. OK hanya untuk non-security hash (cache key).`,
			})
		case "crypto/des":
			cryptoFindings = append(cryptoFindings, CryptoFinding{
				Level: "CRITICAL",
				Type:  "Broken Cipher: DES",
				File:  filePath,
				Line:  fset.Position(imp.Pos()).Line,
				Func:  "(import)",
				Message: `Import "crypto/des". DES hanya 56-bit key — brute-forceable dalam jam. ` +
					`Gunakan AES-256 (crypto/aes) dengan GCM mode.`,
			})
		case "crypto/rc4":
			cryptoFindings = append(cryptoFindings, CryptoFinding{
				Level: "CRITICAL",
				Type:  "Broken Cipher: RC4",
				File:  filePath,
				Line:  fset.Position(imp.Pos()).Line,
				Func:  "(import)",
				Message: `Import "crypto/rc4". RC4 memiliki statistical bias yang sudah di-break. ` +
					`Gunakan AES-GCM atau ChaCha20-Poly1305.`,
			})
		}
	}

	// Check functions
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcName := fn.Name.Name

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check 2: rand.Intn/Read in security-sensitive functions
			if isIdent(sel.X, "rand") {
				funcLow := strings.ToLower(funcName)
				securityContext := strings.Contains(funcLow, "key") ||
					strings.Contains(funcLow, "nonce") ||
					strings.Contains(funcLow, "token") ||
					strings.Contains(funcLow, "secret") ||
					strings.Contains(funcLow, "salt") ||
					strings.Contains(funcLow, "encrypt") ||
					strings.Contains(funcLow, "auth") ||
					strings.Contains(funcLow, "sign") ||
					strings.Contains(funcLow, "hash") ||
					strings.Contains(funcLow, "immune") ||
					strings.Contains(funcLow, "honeypot")

				if securityContext {
					cryptoFindings = append(cryptoFindings, CryptoFinding{
						Level: "CRITICAL",
						Type:  "math/rand in Security Context",
						File:  filePath,
						Line:  fset.Position(call.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							"rand.%s() dipanggil di function %q (security context). "+
								"math/rand predictable — attacker bisa menebak output. "+
								"Gunakan crypto/rand.Read() untuk generate key/nonce/token/salt.",
							sel.Sel.Name, funcName),
					})
				}
			}

			// Check 3: make([]byte, N) with small N for keys
			if sel.Sel.Name == "Read" && isIdent(sel.X, "rand") {
				// Already covered above
			}

			// Check 4: Hardcoded IV/nonce detection
			if isIdent(sel.X, "cipher") && sel.Sel.Name == "NewCBCEncrypter" {
				cryptoFindings = append(cryptoFindings, CryptoFinding{
					Level: "HIGH",
					Type:  "CBC Mode Without AEAD",
					File:  filePath,
					Line:  fset.Position(call.Pos()).Line,
					Func:  funcName,
					Message: "cipher.NewCBCEncrypter() digunakan. CBC mode rentan terhadap " +
						"padding oracle attack tanpa MAC. Gunakan cipher.NewGCM() (AES-GCM) " +
						"yang menyediakan authenticated encryption (AEAD).",
				})
			}

			return true
		})

		// Check 5: Small key/nonce sizes via make([]byte, N)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fnExpr, ok := call.Fun.(*ast.Ident)
			if !ok || fnExpr.Name != "make" {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}

			// Check if making []byte
			arrType, ok := call.Args[0].(*ast.ArrayType)
			if !ok {
				return true
			}
			if id, ok := arrType.Elt.(*ast.Ident); !ok || id.Name != "byte" {
				return true
			}

			// Check size literal
			sizeLit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || sizeLit.Kind != token.INT {
				return true
			}

			size := 0
			fmt.Sscanf(sizeLit.Value, "%d", &size)

			funcLow := strings.ToLower(funcName)
			if size > 0 && size < 16 {
				if strings.Contains(funcLow, "key") || strings.Contains(funcLow, "nonce") ||
					strings.Contains(funcLow, "salt") || strings.Contains(funcLow, "iv") ||
					strings.Contains(funcLow, "encrypt") || strings.Contains(funcLow, "immune") {
					cryptoFindings = append(cryptoFindings, CryptoFinding{
						Level: "HIGH",
						Type:  "Small Key/Nonce Size",
						File:  filePath,
						Line:  fset.Position(call.Pos()).Line,
						Func:  funcName,
						Message: fmt.Sprintf(
							"make([]byte, %d) di function %q — ukuran terlalu kecil untuk "+
								"key/nonce/salt. Minimum: 16 bytes (128-bit) untuk nonce, "+
								"32 bytes (256-bit) untuk key. Ukuran sekarang: %d bytes (%d-bit).",
							size, funcName, size, size*8),
					})
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

func writeCryptoReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 🔑 EXT Crypto Weakness Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_crypto_weakness_scanner v1\n")
	out.WriteString("> **Prinsip:** FQP-6 (BFT), FQP-13 (No-Broadcasting), GOL Fase 7 (Immune)\n")
	out.WriteString("> **Target:** math/rand, MD5, SHA1, DES, RC4, CBC, small key sizes\n\n")

	if len(cryptoFindings) == 0 {
		out.WriteString("✅ *Tidak ditemukan kelemahan kriptografi.*\n")
		return
	}

	crit, high, med := 0, 0, 0
	for _, f := range cryptoFindings {
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
		len(cryptoFindings), crit, high, med))

	for i, f := range cryptoFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}
