// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Agent folder scanner. Audit pass — FLOWORK_AGENTS_DIR env override,
//   0o700 dir perm, continue past invalid entries (anti boot crash), sort
//   deterministic, entry file existence verify, errors wrapped %w.
//
// Agent folder scanner.
//
// Scans ~/.flowork/agents/ (override via FLOWORK_AGENTS_DIR env), parses
// each manifest, and returns a Discovery list. Invalid entries surface as
// rejected with a reason — load attempts continue past failures so one
// broken agent doesn't take down the boot.

package loader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// State — coarse plugin status flag used by the bootstrap loop.
//
type State string

const (
	StateInstalled   State = "installed"    // manifest valid, awaiting boot
	StateBootPending State = "boot_pending" // approved, queued for runtime
	StateReady       State = "ready"        // running, accepting RPC
	StateTombstone   State = "tombstone"    // uninstalled, awaiting GC
	StateFailed      State = "failed"       // boot crash / cap rejected
)

// Discovery — what scanner returns per folder entry. Manifest is non-nil
// only when parse succeeded; RejectReason populated otherwise.
type Discovery struct {
	Path         string
	Manifest     *Manifest
	State        State
	RejectReason string
}

// AgentsDir resolves the agent home directory.
// Priority: FLOWORK_AGENTS_DIR env > ~/.flowork/agents > /tmp/flowork-agents
// (last resort so headless smoke tests still have a writable target).
func AgentsDir() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_AGENTS_DIR")); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".flowork", "agents")
	}
	return "/tmp/flowork-agents"
}

// EnsureDir — create agents dir if missing. 0o700 because manifests can
// contain secret key references and we don't want neighbours peeking.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

// Scan walks the agents directory and returns one Discovery per
// `.fwagent` folder. Non-fwagent entries are silently ignored.
//
// Sort order: folder name asc. Deterministic load order helps reproducible
// dependency resolution later.
func Scan(dir string) ([]Discovery, error) {
	if err := EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("ensure agents dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var out []Discovery
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || !strings.HasSuffix(name, ".fwagent") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		d := Discovery{Path: fullPath}
		manifestPath := filepath.Join(fullPath, "manifest.json")
		raw, rerr := os.ReadFile(manifestPath)
		if rerr != nil {
			d.State = StateFailed
			if errors.Is(rerr, os.ErrNotExist) {
				d.RejectReason = "missing manifest.json"
			} else {
				d.RejectReason = "read manifest: " + rerr.Error()
			}
			out = append(out, d)
			continue
		}
		m, perr := Parse(raw)
		if perr != nil {
			d.State = StateFailed
			d.RejectReason = "parse: " + perr.Error()
			out = append(out, d)
			continue
		}
		// Verify entry file actually exists on disk.
		entryPath := filepath.Join(fullPath, m.Entry)
		if _, ferr := os.Stat(entryPath); ferr != nil {
			d.State = StateFailed
			d.RejectReason = "entry file missing: " + m.Entry
			d.Manifest = m
			out = append(out, d)
			continue
		}
		d.Manifest = m
		d.State = StateInstalled
		out = append(out, d)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}
