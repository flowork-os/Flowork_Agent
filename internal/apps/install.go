// install.go — ROADMAP 4: plug-and-play install/uninstall/hot-reload app (ala Android).
//
// App pack = .fwpack (zip): plugin.json {"kind":"app","id":"<id>"} + apps/<id>/{manifest.json,
// core.*, ui/*}. Install → extract STAGING → atomic rename ke <appsDir>/<id> → reload SATU app
// (daftarin tool + siap dipakai GUI) TANPA restart. Uninstall → stop core + unregister tool +
// hapus folder. Isolasi total: 1 app = 1 folder, ga ada sisa di tempat lain.
//
// KEAMANAN: core_entry app = perintah OS (exec bebas, lintas-bahasa). Karena itu install lewat
// gerbang owner WAJIB consent (approveExec) — persis danger-caps di agent pack. Owner yang buka
// gerbang, bukan AI.
package apps

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/tools"
)

// defaultMgr — singleton manager yang dipakai package-level Install/Uninstall (dipanggil dari
// gerbang seragam plugin_handler + HTTP handler). Di-set sekali saat boot via SetDefault.
var defaultMgr *Manager

// SetDefault — daftarkan manager hidup sebagai target install/uninstall package-level.
func SetDefault(m *Manager) { defaultMgr = m }

// unregisterTools — copot semua tool app dari registry global (kebalikan registerTools).
func (m *Manager) unregisterTools(id string) {
	m.mu.Lock()
	names := m.regs[id]
	delete(m.regs, id)
	m.mu.Unlock()
	for _, n := range names {
		_ = tools.Unregister(n)
	}
}

// stopProc — matikan core app kalau lagi jalan (dipakai uninstall/reload biar proc lama mati).
func (m *Manager) stopProc(id string) {
	m.mu.Lock()
	p := m.procs[id]
	delete(m.procs, id)
	m.mu.Unlock()
	if p != nil {
		p.close()
	}
}

// reloadOne — baca SATU app dari <dir>/<id>/manifest.json, daftarkan (tool + registry). Dipakai
// setelah install: app langsung LIVE tanpa restart. Reinstall = unregister tool lama dulu.
func (m *Manager) reloadOne(id string) error {
	if !appIDRe.MatchString(id) {
		return errors.New("app id invalid")
	}
	raw, err := os.ReadFile(filepath.Join(m.dir, id, "manifest.json"))
	if err != nil {
		return errors.New("manifest.json tak terbaca: " + err.Error())
	}
	var man Manifest
	if json.Unmarshal(raw, &man) != nil || man.Kind != "app" || !appIDRe.MatchString(man.ID) || man.ID != id {
		return errors.New("manifest invalid (kind harus 'app', id cocok folder)")
	}
	// app yang sama mungkin sudah live (reinstall) → bersihin proc + tool lama dulu.
	m.stopProc(id)
	m.unregisterTools(id)
	app := &App{Manifest: man, Dir: filepath.Join(m.dir, id)}
	m.mu.Lock()
	m.apps[man.ID] = app
	m.mu.Unlock()
	m.registerTools(app)
	return nil
}

// Uninstall — copot app total: stop core, unregister tool, buang dari registri, hapus folder.
func (m *Manager) Uninstall(id string) error {
	if !appIDRe.MatchString(id) {
		return errors.New("app id invalid")
	}
	m.mu.Lock()
	_, ok := m.apps[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("app tak ditemukan: " + id)
	}
	m.stopProc(id)
	m.unregisterTools(id)
	m.mu.Lock()
	delete(m.apps, id)
	delete(m.version, id)
	m.mu.Unlock()
	return os.RemoveAll(filepath.Join(m.dir, id))
}

// installFromZip — extract app pack ke staging → atomic rename → reloadOne. approveExec WAJIB
// true (core = exec bebas). Balik (body, http-status); status 0 = sukses (gerbang konversi ke 200).
func (m *Manager) installFromZip(raw []byte, approveExec bool) (map[string]any, int) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return map[string]any{"error": "not a valid zip: " + err.Error()}, http.StatusBadRequest
	}
	// id dari plugin.json (gerbang seragam) ATAU dari apps/<id>/manifest.json.
	id := ""
	for _, f := range zr.File {
		base := strings.TrimPrefix(f.Name, "./")
		if base == "plugin.json" || strings.HasSuffix(base, "/plugin.json") {
			if rc, e := f.Open(); e == nil {
				pj, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
				rc.Close()
				var p struct{ Kind, ID string }
				if json.Unmarshal(pj, &p) == nil {
					if p.Kind != "" && p.Kind != "app" {
						return map[string]any{"error": "kind bukan 'app'"}, http.StatusBadRequest
					}
					id = strings.TrimSpace(p.ID)
				}
			}
			break
		}
	}
	if id == "" { // fallback: tebak dari path apps/<id>/manifest.json
		for _, f := range zr.File {
			name := strings.TrimPrefix(f.Name, "./")
			if strings.HasPrefix(name, "apps/") && strings.HasSuffix(name, "/manifest.json") {
				rest := strings.TrimPrefix(name, "apps/")
				if i := strings.IndexByte(rest, '/'); i > 0 {
					id = rest[:i]
				}
				break
			}
		}
	}
	if !appIDRe.MatchString(id) {
		return map[string]any{"error": "app id invalid / tak ketemu di pack (^[a-z0-9][a-z0-9-]{1,40}$)"}, http.StatusBadRequest
	}
	if !approveExec {
		return map[string]any{
			"error":            "app menjalankan perintah OS (core lintas-bahasa) — butuh persetujuan owner",
			"consent_required": true,
			"approve_hint":     "install ulang dengan ?approve_exec=1 kalau lo percaya app ini",
			"app":              id,
		}, http.StatusForbidden
	}

	staging := filepath.Join(m.dir, ".app-staging-"+id)
	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging)
	prefix := "apps/" + id + "/"
	got, hasManifest := 0, false
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if !strings.HasPrefix(name, prefix) || strings.HasSuffix(name, "/") {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		dest := filepath.Join(staging, filepath.FromSlash(rel))
		if c, e := filepath.Rel(staging, dest); e != nil || strings.HasPrefix(c, "..") {
			continue // anti zip-slip
		}
		if e := os.MkdirAll(filepath.Dir(dest), 0o755); e != nil {
			return map[string]any{"error": "mkdir: " + e.Error()}, http.StatusInternalServerError
		}
		rc, e := f.Open()
		if e != nil {
			continue
		}
		out, e := os.Create(dest)
		if e != nil {
			rc.Close()
			return map[string]any{"error": "create: " + e.Error()}, http.StatusInternalServerError
		}
		_, _ = io.Copy(out, io.LimitReader(rc, 64<<20))
		out.Close()
		rc.Close()
		got++
		if rel == "manifest.json" {
			hasManifest = true
		}
	}
	if !hasManifest {
		return map[string]any{"error": "pack ga punya apps/" + id + "/manifest.json"}, http.StatusBadRequest
	}

	// atomic rename staging → final, lalu reload SATU app (LIVE tanpa restart).
	final := filepath.Join(m.dir, id)
	if e := os.MkdirAll(m.dir, 0o755); e != nil {
		return map[string]any{"error": "mkdir appsdir: " + e.Error()}, http.StatusInternalServerError
	}
	_ = os.RemoveAll(final)
	if e := os.Rename(staging, final); e != nil {
		return map[string]any{"error": "install: " + e.Error()}, http.StatusInternalServerError
	}
	if e := m.reloadOne(id); e != nil {
		_ = os.RemoveAll(final) // rollback biar ga ada app rusak nyangkut
		return map[string]any{"error": "reload: " + e.Error()}, http.StatusBadRequest
	}
	return map[string]any{"ok": true, "app": id, "files": got, "next": "app LIVE — buka di tab App."}, 0
}

// InstallAppPack — entry package-level (gerbang seragam case "app" + HTTP /api/apps/install).
func InstallAppPack(raw []byte, approveExec bool) (map[string]any, int) {
	if defaultMgr == nil {
		return map[string]any{"error": "apps manager belum siap"}, http.StatusInternalServerError
	}
	return defaultMgr.installFromZip(raw, approveExec)
}

// UninstallApp — entry package-level (HTTP /api/apps/uninstall).
func UninstallApp(id string) error {
	if defaultMgr == nil {
		return errors.New("apps manager belum siap")
	}
	return defaultMgr.Uninstall(id)
}
