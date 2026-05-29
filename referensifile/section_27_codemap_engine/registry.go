// Package codeindex — registry.go
//
// GitNexus-inspired: Multi-repo unified graph registry.
// Track semua repo yang sudah di-index, enable cross-repo dependency analysis.
package codeindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RepoEntry — satu repo yang terdaftar di registry.
type RepoEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	LastIndex time.Time `json:"last_index"`
	NodeCount int       `json:"node_count"`
	EdgeCount int       `json:"edge_count"`
	FuncNodes int       `json:"func_nodes"`
	FuncEdges int       `json:"func_edges"`
}

// Registry — multi-repo registry.
type Registry struct {
	Repos   []RepoEntry `json:"repos"`
	Updated time.Time   `json:"updated"`
}

const registryFile = "codemap-registry.json"

// registryPath returns full path to registry file.
func registryPath(stateDir string) string {
	return filepath.Join(stateDir, "state", registryFile)
}

// LoadRegistry — baca registry dari disk.
func LoadRegistry(workspaceRoot string) (*Registry, error) {
	path := registryPath(workspaceRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return &Registry{}, nil // corrupt = start fresh
	}
	return &reg, nil
}

// SaveRegistry — tulis registry ke disk.
func SaveRegistry(workspaceRoot string, reg *Registry) error {
	reg.Updated = time.Now()
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	path := registryPath(workspaceRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// RegisterRepo — tambah atau update repo di registry.
func RegisterRepo(workspaceRoot string, entry RepoEntry) error {
	reg, err := LoadRegistry(workspaceRoot)
	if err != nil {
		return err
	}

	// Update if exists
	found := false
	for i, r := range reg.Repos {
		if r.Name == entry.Name || r.Path == entry.Path {
			reg.Repos[i] = entry
			found = true
			break
		}
	}
	if !found {
		reg.Repos = append(reg.Repos, entry)
	}

	return SaveRegistry(workspaceRoot, reg)
}

// UnregisterRepo — hapus repo dari registry.
func UnregisterRepo(workspaceRoot string, name string) error {
	reg, err := LoadRegistry(workspaceRoot)
	if err != nil {
		return err
	}

	var filtered []RepoEntry
	for _, r := range reg.Repos {
		if r.Name != name {
			filtered = append(filtered, r)
		}
	}
	reg.Repos = filtered
	return SaveRegistry(workspaceRoot, reg)
}

// ListRepos — return semua repo yang terdaftar.
func ListRepos(workspaceRoot string) ([]RepoEntry, error) {
	reg, err := LoadRegistry(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return reg.Repos, nil
}

// RepoIDFromPath — generate stable repo ID dari path.
func RepoIDFromPath(repoPath string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	// Use folder name as repo ID (simple, readable)
	return filepath.Base(abs)
}

// DetectSiblingRepos — auto-detect repo lain di parent directory.
// Cari folder yang punya .git/ di level yang sama.
func DetectSiblingRepos(workspaceRoot string) ([]RepoEntry, error) {
	parentDir := filepath.Dir(workspaceRoot)
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil, fmt.Errorf("read parent dir: %w", err)
	}

	var repos []RepoEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidatePath := filepath.Join(parentDir, e.Name())
		gitDir := filepath.Join(candidatePath, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			repos = append(repos, RepoEntry{
				ID:   RepoIDFromPath(candidatePath),
				Name: e.Name(),
				Path: candidatePath,
			})
		}
	}

	return repos, nil
}
