//go:build ignore

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SGVP Dockerfile Security Auditor
// Mendeteksi bahaya seperti USER root dan ENV hardcoded token credential di dalam instansiasi Dockerfile.
func main() {
	cwd, _ := os.Getwd()
	repoRoot := getRepoRoot(cwd)

	findings := 0
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.Contains(strings.ToLower(d.Name()), "dockerfile") {
			return nil
		}

		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		lines := strings.Split(string(raw), "\n")
		hasUserDef := false
		for i, line := range lines {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToUpper(l), "USER ") {
				if strings.Contains(strings.ToLower(l), "root") {
					rel, _ := filepath.Rel(repoRoot, path)
					fmt.Printf("[CRITICAL] Dockerfile hazard: Tidak aman menggunakan root permission secara eksplisit di %s baris %d\n", rel, i+1)
					findings++
				}
				hasUserDef = true
			}
			if strings.HasPrefix(strings.ToUpper(l), "ENV ") {
				if strings.Contains(strings.ToLower(l), "token") || strings.Contains(strings.ToLower(l), "password") || strings.Contains(strings.ToLower(l), "secret") {
					rel, _ := filepath.Rel(repoRoot, path)
					fmt.Printf("[CRITICAL] Dockerfile hazard: Hardcoded token credential env ditemukan di %s baris %d\n", rel, i+1)
					findings++
				}
			}
		}

		if !hasUserDef {
			rel, _ := filepath.Rel(repoRoot, path)
			fmt.Printf("[HIGH] Dockerfile hazard: Tidak mendefinisikan environment layer permission user none-root di %s\n", rel)
			findings++
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking repo: %v\n", err)
		os.Exit(1)
	}

	if findings > 0 {
		fmt.Printf("🚨 Ditemukan %d Dockerfile Security hazard(s).\n", findings)
		os.Exit(1)
	}
}

func getRepoRoot(cwd string) string {
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return cwd
}
