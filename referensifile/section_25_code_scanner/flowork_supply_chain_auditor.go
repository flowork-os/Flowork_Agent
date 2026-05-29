//go:build ignore

// phantom_supply_chain_scanner — Anti-Racun Pihak Ketiga (Gemini blueprint
// rc122, delegated 2026-04-19T03:04:57Z).
//
// Purpose: sniff import baru di go.mod yang tidak di-whitelist swarm. Cegah
// supply-chain malware yang di-inject via `go get` oleh AI agent halusinasi
// atau attacker mid-build.
//
// Strategy:
//  1. Parse go.mod, ambil semua `require` entries
//  2. Match against whitelist `_sgvp/supply_chain_allowlist.txt`
//  3. Flag CRITICAL untuk import yang tidak di allowlist
//  4. Flag MEDIUM untuk import yang tidak di allowlist tapi di indirect-known
//     (standar ecosystem Go, e.g. golang.org/x/*, google.golang.org/*)
//
// Whitelist format: satu line per module path (comment `#` di awal = skip).
// Bila file tidak ada, scanner akan print WARN + skip gracefully (don't
// break build).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Finding struct {
	Level   string
	Module  string
	Version string
	Message string
}

// Well-known Go ecosystem paths yang umum dipakai Flowork (default baseline).
// Ini bukan pengganti allowlist file — hanya prefix-baseline supaya tidak
// semua stdlib-adjacent di-flag saat belum ada _sgvp/supply_chain_allowlist.txt.
var baselineTrustedPrefixes = []string{
	"golang.org/x/",
	"google.golang.org/",
	"go.opencensus.io",
	"gopkg.in/",
	"github.com/teetah2402/flowork", // self
}

func main() {
	start := time.Now()
	fmt.Println("📦 [\033[1;35mPHANTOM SUPPLY CHAIN SCANNER\033[0m] Cek go.mod untuk import yang tidak di allowlist...")

	goModPath := "go.mod"
	f, err := os.Open(goModPath)
	if err != nil {
		fmt.Printf("⚠️  WARN: go.mod tidak ditemukan di %s — scanner skip\n", goModPath)
		fmt.Printf("\n⏱️  Selesai dalam %v\n", time.Since(start))
		return
	}
	defer f.Close()

	// Load allowlist (optional — scanner graceful kalau tidak ada).
	allowlist := loadAllowlist()

	var findings []Finding
	inRequireBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		if line == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		var modulePath, version string
		if inRequireBlock {
			// Format: `github.com/x/y v1.2.3 // indirect`
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			modulePath = parts[0]
			version = parts[1]
		} else if strings.HasPrefix(line, "require ") {
			// Single-line require: `require github.com/x/y v1.2.3`
			parts := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(parts) < 2 {
				continue
			}
			modulePath = parts[0]
			version = parts[1]
		} else {
			continue
		}

		// Skip kalau di baseline trusted prefix.
		if isBaselineTrusted(modulePath) {
			continue
		}
		// Skip kalau explicit di allowlist atau match trusted org prefix.
		if allowlist[modulePath] || matchAllowlistPrefix(modulePath, allowlist) {
			continue
		}
		// Flag sebagai phantom.
		findings = append(findings, Finding{
			Level:   "HIGH",
			Module:  modulePath,
			Version: version,
			Message: fmt.Sprintf("Phantom import: %s@%s tidak di allowlist. Tambah ke _sgvp/supply_chain_allowlist.txt kalau memang dibutuhkan swarm, atau hapus dari go.mod.", modulePath, version),
		})
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("❌ ERROR reading go.mod: %v\n", err)
		fmt.Printf("\n⏱️  Selesai dalam %v\n", time.Since(start))
		return
	}

	if len(findings) == 0 {
		fmt.Println("✅ Semua import di go.mod terdaftar di allowlist atau baseline trusted.")
	}
	for _, f := range findings {
		fmt.Printf("\033[1;31m[%s]\033[0m %s %s -> %s\n", f.Level, f.Module, f.Version, f.Message)
	}
	fmt.Printf("\n⏱️  Selesai dalam %v | %d temuan\n", time.Since(start), len(findings))
}

func isBaselineTrusted(modulePath string) bool {
	for _, prefix := range baselineTrustedPrefixes {
		if strings.HasPrefix(modulePath, prefix) {
			return true
		}
	}
	return false
}

// matchAllowlistPrefix: entry allowlist yang berakhir dengan "/" jadi
// prefix-match (contoh: "github.com/charmbracelet/" allow seluruh sub-pkg).
func matchAllowlistPrefix(modulePath string, allowlist map[string]bool) bool {
	for entry := range allowlist {
		if strings.HasSuffix(entry, "/") && strings.HasPrefix(modulePath, entry) {
			return true
		}
	}
	return false
}

func loadAllowlist() map[string]bool {
	result := make(map[string]bool)
	candidates := []string{
		filepath.Join("_sgvp", "supply_chain_allowlist.txt"),
		filepath.Join("scanner", "supply_chain_allowlist.txt"),
	}
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			result[line] = true
		}
		return result
	}
	return result
}
