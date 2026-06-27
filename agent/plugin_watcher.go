// plugin_watcher.go — Plug-and-Play Phase 3: drop-folder auto-install.
//
// Taruh .fwpack di ~/.flowork/dropbox/ → AUTO-INSTALL. Owner naruh sendiri =
// trusted → auto-approve caps, TAPI caps yang di-grant di-LOG (jejak awareness).
// Pack pindah ke dropbox/installed/ (sukses) atau dropbox/failed/ (gagal).
//
// Poll 4s + "settled" check (mtime > 2s) biar ga baca file yang lagi di-copy
// (partial). Poll (bukan fsnotify) = simpel + robust buat use-case ini.

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

// pluginDropboxDir — ~/.flowork/dropbox (sibling AgentsDir, portable).
func pluginDropboxDir() string {
	return filepath.Join(filepath.Dir(loader.AgentsDir()), "dropbox")
}

// startPluginDropWatcher — poll dropbox, auto-install .fwpack yang masuk.
func startPluginDropWatcher(host *kernelhost.Host, store *floworkdb.Store) {
	inbox := pluginDropboxDir()
	done := filepath.Join(inbox, "installed")
	failed := filepath.Join(inbox, "failed")
	for _, d := range []string{inbox, done, failed} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Printf("[plugin-drop] mkdir %s: %v", d, err)
			return
		}
	}
	go func() {
		log.Printf("[plugin-drop] watcher armed on %s (drop .fwpack → auto-install)", inbox)
		for {
			time.Sleep(4 * time.Second)
			entries, err := os.ReadDir(inbox)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".fwpack") {
					continue
				}
				info, err := e.Info()
				if err != nil || time.Since(info.ModTime()) < 2*time.Second {
					continue // belum settled (mungkin lagi di-copy)
				}
				path := filepath.Join(inbox, e.Name())
				raw, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				// owner naruh sendiri = trusted → auto-approve caps (tapi di-log).
				// TAPI verdict VERIFIER "blocked" (pola jahat) TETAP dihormati (override=false)
				// — drop-folder bukan izin paksa pola jahat (ROADMAP_AI_STUDIO F2 gerbang wajib).
				res := installPluginPack(host, store, raw, true, false)
				dst := done
				if res.status != 0 {
					dst = failed
				}
				caps, _ := res.body["dangerous_caps"].([]string)
				log.Printf("[plugin-drop] %s → %s | plugin=%v category=%v smoke=%v caps_granted=%v err=%v",
					e.Name(), filepath.Base(dst), res.body["plugin"], res.body["category"],
					res.body["smoke"], caps, res.body["error"])
				if err := os.Rename(path, filepath.Join(dst, e.Name())); err != nil {
					log.Printf("[plugin-drop] move %s: %v", e.Name(), err)
				}
			}
		}
	}()
}
