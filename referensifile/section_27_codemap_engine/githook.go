// Package codeindex — githook.go
//
// CRG-inspired git hook integration.
// Install post-commit hook yang auto-trigger incremental reindex.
// Prinsip: "incremental updates in <2 seconds" setelah setiap commit.
package codeindex

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const hookScript = `#!/bin/sh
# FloworkOS codeindex — auto-incremental update setelah commit.
# Installed by FloworkOS GUI Code Map.
# Prinsip dari code-review-graph: keep graph fresh on every commit.

echo "[flowork-codeindex] post-commit hook: triggering incremental reindex..."

# Fire-and-forget HTTP POST ke GUI server (jika running)
if command -v curl > /dev/null 2>&1; then
    curl -s -X POST "http://localhost:3101/api/codemap/reindex?mode=incremental" > /dev/null 2>&1 &
fi
`

const hookScriptWindows = `@echo off
REM FloworkOS codeindex — auto-incremental update setelah commit.
REM Installed by FloworkOS GUI Code Map.

echo [flowork-codeindex] post-commit hook: triggering incremental reindex...

REM Fire-and-forget HTTP POST ke GUI server (jika running)
start /b curl -s -X POST "http://localhost:3101/api/codemap/reindex?mode=incremental" >nul 2>&1
`

// InstallGitHook pasang post-commit hook di .git/hooks/ yang trigger incremental reindex.
// Safe: tidak overwrite hook yang sudah ada, append saja.
func InstallGitHook(workspaceRoot string) error {
	gitDir := filepath.Join(workspaceRoot, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("bukan git repo: %s", workspaceRoot)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("mkdir hooks: %w", err)
	}

	hookFile := filepath.Join(hooksDir, "post-commit")
	if runtime.GOOS == "windows" {
		hookFile = filepath.Join(hooksDir, "post-commit.bat")
	}

	// Cek apakah hook sudah ada
	existing, _ := os.ReadFile(hookFile)
	if strings.Contains(string(existing), "flowork-codeindex") {
		log.Printf("[githook] hook sudah terpasang di %s", hookFile)
		return nil // sudah ada, skip
	}

	script := hookScript
	if runtime.GOOS == "windows" {
		script = hookScriptWindows
	}

	// Kalau file sudah ada (hook lain), append
	if len(existing) > 0 {
		script = string(existing) + "\n\n# --- FloworkOS codeindex hook (appended) ---\n" + script
	}

	if err := os.WriteFile(hookFile, []byte(script), 0755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}

	log.Printf("[githook] installed post-commit hook: %s", hookFile)
	return nil
}

// UninstallGitHook hapus FloworkOS section dari post-commit hook.
func UninstallGitHook(workspaceRoot string) error {
	hookFile := filepath.Join(workspaceRoot, ".git", "hooks", "post-commit")
	if runtime.GOOS == "windows" {
		hookFile = filepath.Join(workspaceRoot, ".git", "hooks", "post-commit.bat")
	}

	data, err := os.ReadFile(hookFile)
	if err != nil {
		return nil // hook tidak ada, skip
	}

	content := string(data)
	if !strings.Contains(content, "flowork-codeindex") {
		return nil // bukan hook kita
	}

	// Hapus section FloworkOS
	idx := strings.Index(content, "# --- FloworkOS codeindex hook")
	if idx > 0 {
		content = strings.TrimRight(content[:idx], "\n")
		if strings.TrimSpace(content) == "" {
			os.Remove(hookFile) // file kosong, hapus aja
		} else {
			os.WriteFile(hookFile, []byte(content), 0755)
		}
	} else {
		// Entire file is our hook — remove
		os.Remove(hookFile)
	}

	log.Printf("[githook] uninstalled post-commit hook: %s", hookFile)
	return nil
}
