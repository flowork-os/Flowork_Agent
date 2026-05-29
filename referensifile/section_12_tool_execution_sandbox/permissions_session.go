package tools

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/fsutil"
)

// ─── Session allowlist (persisted) ──────────────────────────────

type allowlist struct {
	Tools    []string            `json:"tools,omitempty"`
	Commands map[string][]string `json:"commands,omitempty"`
}

func allowPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flowork", "allowed.json")
}

func loadAllowlist() allowlist {
	p := allowPath()
	if p == "" {
		return allowlist{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return allowlist{}
	}
	var a allowlist
	if err := json.Unmarshal(data, &a); err != nil {
		fmt.Fprintf(os.Stderr, "permissions: corrupt allowlist, resetting: %v\n", err)
	}
	return a
}

// BUG-H16 fix (2026-04-19): cache allowlist in-memory, invalidate by mtime.
// sessionAllowed dipanggil untuk SETIAP tool call — tanpa cache hot path baca
// file dari disk 10-100ms per call di Windows (antivirus scan). Cache pake
// RWMutex + mtime check sehingga perubahan via `sessionAllow` atau edit file
// manual ter-pickup tanpa restart.
var (
	allowCacheMu   sync.RWMutex
	allowCacheData allowlist
	allowCacheMTim time.Time
	allowCachePath string
)

func loadAllowlistCached() allowlist {
	p := allowPath()
	if p == "" {
		return allowlist{}
	}
	info, err := os.Stat(p)
	if err != nil {
		allowCacheMu.Lock()
		allowCacheData = allowlist{}
		allowCacheMTim = time.Time{}
		allowCachePath = p
		allowCacheMu.Unlock()
		return allowlist{}
	}
	mt := info.ModTime()
	allowCacheMu.RLock()
	if allowCachePath == p && allowCacheMTim.Equal(mt) {
		cached := allowCacheData
		allowCacheMu.RUnlock()
		return cached
	}
	allowCacheMu.RUnlock()

	a := loadAllowlist()
	allowCacheMu.Lock()
	allowCacheData = a
	allowCacheMTim = mt
	allowCachePath = p
	allowCacheMu.Unlock()
	return a
}

func sessionAllowed(inv *Invocation) bool {
	a := loadAllowlistCached()
	for _, t := range a.Tools {
		if t == inv.ToolName {
			return true
		}
	}
	if hashes, ok := a.Commands[inv.ToolName]; ok {
		h := hashInvocation(inv)
		for _, hash := range hashes {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(h)) == 1 {
				return true
			}
		}
	}
	return false
}

// allowlistMu serialises sessionAllow writes within a single process so
// two goroutines racing to persist different tools don't overwrite each
// other's update. Cross-process races are mitigated by atomic tmp+rename
// below (partial writes are never visible).
//
// CODEX-BUG-15 fix: the original read-modify-write had neither an
// in-process lock nor an atomic replace — running flowork TUI + a
// background watcher simultaneously could lose whichever update lost the
// race. The combination below gives strong single-process consistency and
// at least "last writer wins cleanly" across processes.
var allowlistMu sync.Mutex

func sessionAllow(tool, cmdHash string) error {
	p := allowPath()
	if p == "" {
		return fmt.Errorf("no home dir")
	}
	allowlistMu.Lock()
	defer allowlistMu.Unlock()

	a := loadAllowlist()
	if cmdHash == "" {
		for _, t := range a.Tools {
			if t == tool {
				return nil
			}
		}
		a.Tools = append(a.Tools, tool)
	} else {
		if a.Commands == nil {
			a.Commands = make(map[string][]string)
		}
		hashes := a.Commands[tool]
		for _, h := range hashes {
			if h == cmdHash {
				return nil
			}
		}
		a.Commands[tool] = append(a.Commands[tool], cmdHash)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir allowlist dir: %w", err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal allowlist: %w", err)
	}
	// BUG-046 fix (2026-04-25): SafeWriteFile routes through fsutil's
	// traversal guard. Drops the explicit tmp+rename pair — fsutil is
	// the project-wide consistent boundary for writes.
	if err := fsutil.SafeWriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("write allowlist: %w", err)
	}
	return nil
}

// hashInvocation generates a canonical SHA256 hash for an invocation's core arguments.
func hashInvocation(inv *Invocation) string {
	if inv == nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(inv.ToolName))
	h.Write([]byte{0})

	if inv.ParsedArgs != nil {
		if cmd, ok := inv.ParsedArgs["command"].(string); ok {
			h.Write([]byte(cmd))
		} else if path, ok := inv.ParsedArgs["path"].(string); ok {
			h.Write([]byte(path))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// isInteractiveStdin reports whether os.Stdin is a character device (TTY).
// Portable check using os.File.Stat mode; no x/term dep needed here.
func isInteractiveStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
