// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

var (
	trackedMu sync.Mutex
	tracked   = map[string]bool{}
)

func trackName(name string) {
	trackedMu.Lock()
	defer trackedMu.Unlock()
	tracked[strings.ToLower(name)] = true
}

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

func TrackedNames() []string {
	trackedMu.Lock()
	defer trackedMu.Unlock()
	out := make([]string, 0, len(tracked))
	for k := range tracked {
		out = append(out, k)
	}
	return out
}

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

func snapshotRegistry() map[string]bool {
	out := map[string]bool{}
	for _, s := range slashcmd.ListSummaries() {
		out[strings.ToLower(s.Name)] = true
	}
	return out
}

func LoadFromDirs(dirs []string) (totalLoaded, totalSkipped int, lastErr error) {

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

func Reload(dirs []string) (loaded, skipped int) {
	ClearAll()
	loaded, skipped, err := LoadFromDirs(dirs)
	if err != nil {
		log.Printf("[custom-slash] reload error: %v", err)
	}
	log.Printf("[custom-slash] reload: loaded=%d skipped=%d", loaded, skipped)
	return loaded, skipped
}

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
