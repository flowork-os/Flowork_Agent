// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 16 phase 2 (Hot-reload + multi-warga). API stable:
//   StartWatcher (fsnotify), LoadFromDirs (multi-warga), TrackedNames.
//   Loader keeps internal track of which commands are custom-sourced
//   biar bisa Unregister sebelum reload. Phase 3 (run:llm frontmatter
//   → tambah file baru llm_runner.go, JANGAN modify ini).
//
// watcher.go — Section 16 phase 2: fsnotify hot-reload + multi-warga
// commands dir support.

package custom

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/slashcmd"

	"github.com/fsnotify/fsnotify"
)

// trackedMu guards tracked map yang record nama command yang loader
// register. Pakai untuk Unregister-then-Reload tanpa nyentuh built-in
// commands.
var (
	trackedMu sync.Mutex
	tracked   = map[string]bool{} // canonical name → true
)

// trackName — mark name as custom-source-controlled.
func trackName(name string) {
	trackedMu.Lock()
	defer trackedMu.Unlock()
	tracked[strings.ToLower(name)] = true
}

// untrackName — strip + return true kalau previously tracked.
func untrackName(name string) bool {
	trackedMu.Lock()
	defer trackedMu.Unlock()
	name = strings.ToLower(name)
	if !tracked[name] {
		return false
	}
	delete(tracked, name)
	return true
}

// TrackedNames — snapshot. Untuk diagnostic.
func TrackedNames() []string {
	trackedMu.Lock()
	defer trackedMu.Unlock()
	out := make([]string, 0, len(tracked))
	for k := range tracked {
		out = append(out, k)
	}
	return out
}

// ClearAll — unregister all tracked custom commands. Idempotent.
func ClearAll() {
	trackedMu.Lock()
	names := make([]string, 0, len(tracked))
	for k := range tracked {
		names = append(names, k)
	}
	trackedMu.Unlock()
	for _, n := range names {
		slashcmd.Unregister(n)
		untrackName(n)
	}
}

// snapshotRegistry — return set of currently registered canonical names.
// Pakai untuk diff tracking di LoadFromDirs (locked loader.go ngga panggil
// trackName — kita inject post-load).
func snapshotRegistry() map[string]bool {
	out := map[string]bool{}
	for _, s := range slashcmd.ListSummaries() {
		out[strings.ToLower(s.Name)] = true
	}
	return out
}

// LoadFromDirs — multi-warga loader. Iterate dirs slice, panggil
// LoadFromDir per dir, akumulasi total loaded + skipped count.
//
// Pattern caller (main.go):
//
//	dirs := []string{}
//	for _, agentID := range allAgentIDs {
//	    if shared, err := host.SharedDirForAgent(agentID); err == nil {
//	        dirs = append(dirs, filepath.Join(shared, "commands"))
//	    }
//	}
//	custom.LoadFromDirs(dirs)
func LoadFromDirs(dirs []string) (totalLoaded, totalSkipped int, lastErr error) {
	// Snapshot sebelum load — sehingga setelah load, diff = newly registered
	// command yang harus di-track (anti unregister built-in).
	before := snapshotRegistry()
	for _, dir := range dirs {
		loaded, skipped, err := LoadFromDir(dir)
		totalLoaded += loaded
		totalSkipped += skipped
		if err != nil {
			lastErr = err
		}
	}
	after := snapshotRegistry()
	for name := range after {
		if !before[name] {
			trackName(name)
		}
	}
	return
}

// Reload — clear tracked custom commands + re-scan all `dirs`. Used by
// watcher when files change. Logs result.
func Reload(dirs []string) (loaded, skipped int) {
	ClearAll()
	loaded, skipped, err := LoadFromDirs(dirs)
	if err != nil {
		log.Printf("[custom-slash] reload error: %v", err)
	}
	log.Printf("[custom-slash] reload: loaded=%d skipped=%d", loaded, skipped)
	return loaded, skipped
}

// StartWatcher — fsnotify watcher across all dirs. Debounce 500ms supaya
// burst write (editor save = multiple events) coalesce ke single Reload.
//
// ctx cancellation → stop watcher + close fsnotify.Watcher.
//
// Skip dirs yang ngga exist (best-effort — anti error kalau warga ngga
// punya commands/ folder).
func StartWatcher(ctx context.Context, dirs []string) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	added := 0
	for _, d := range dirs {
		if err := w.Add(d); err != nil {
			log.Printf("[custom-slash] watch add %s: %v (skipped)", d, err)
			continue
		}
		added++
	}
	if added == 0 {
		_ = w.Close()
		log.Printf("[custom-slash] no commands dirs to watch")
		return nil
	}
	log.Printf("[custom-slash] watching %d commands dirs", added)

	go func() {
		defer w.Close()
		debounce := time.NewTimer(time.Hour)
		debounce.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				// Only care about .md changes (create/write/remove/rename).
				if filepath.Ext(strings.ToLower(ev.Name)) != ".md" {
					continue
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
					continue
				}
				debounce.Reset(500 * time.Millisecond)
			case <-debounce.C:
				Reload(dirs)
			case werr, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Printf("[custom-slash] watcher error: %v", werr)
			}
		}
	}()
	return nil
}
