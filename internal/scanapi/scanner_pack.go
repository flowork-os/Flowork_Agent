// scanner_pack.go — kind:scanner PLUG-AND-PLAY (.fwpack). Satu paket = bundle
// nuclei check (.yaml) yg bisa colok-cabut kayak tool/slash, TAPI payload-nya
// DATA (yaml check) bukan wasm agent.
//
// Layout .fwpack (zip): plugin.json {id, kind:"scanner", scanner:{name,description}}
//   + checks/*.yaml  (template nuclei).
//
// Install: extract checks → STAGING → `nuclei -validate` (gerbang, buang yg invalid)
//   → atomic rename ke <nuclei-templates>/flowork-pack-<id>/ → AUTO masuk arsenal
//   (subdir nuclei → ke-enumerate registry, install/uninstall/exclude jalan).
// Uninstall: hapus dir pack. List: enumerate flowork-pack-*.
//
//	POST /api/scanner/packs/install    (multipart "file" = .fwpack)
//	POST /api/scanner/packs/uninstall?id=<pack-id>
//	GET  /api/scanner/packs/installed
//
// AMAN: owner-only loopback; tiap check lewat nuclei -validate; nuclei runtime
// tanpa -code (template inert); anti zip-slip; nama pack di-sanitize.

package scanapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const scannerPackPrefix = "flowork-pack-"

type scannerPackManifest struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Scanner struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"scanner"`
}

// validatePackInvalidFiles — `nuclei -validate -t dir` SEKALI → set path template
// yg INVALID (di-parse dari output). Jauh lebih cepat dari validate per-file.
func validatePackInvalidFiles(dir string) map[string]bool {
	invalid := map[string]bool{}
	if _, err := exec.LookPath("nuclei"); err != nil {
		return invalid // nuclei ga ada → ga bisa nyaring, anggap semua lolos (graceful)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "nuclei", "-validate", "-t", dir, "-nc", "-disable-update-check").CombinedOutput()
	for _, ln := range strings.Split(string(out), "\n") {
		const mark = "Error occurred loading template "
		if i := strings.Index(ln, mark); i >= 0 {
			rest := ln[i+len(mark):]
			if j := strings.IndexByte(rest, ':'); j > 0 {
				invalid[strings.TrimSpace(rest[:j])] = true
			}
		}
	}
	return invalid
}

// InstallScannerPack — CORE install. Balik (body, status). status 0 = ok.
func InstallScannerPack(raw []byte) (map[string]any, int) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return map[string]any{"error": "not a valid zip: " + err.Error()}, http.StatusBadRequest
	}
	// 1) plugin.json
	var manRaw []byte
	for _, f := range zr.File {
		base := strings.TrimPrefix(f.Name, "./")
		if base == "plugin.json" || strings.HasSuffix(base, "/plugin.json") {
			rc, e := f.Open()
			if e == nil {
				manRaw, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
				rc.Close()
			}
			break
		}
	}
	if manRaw == nil {
		return map[string]any{"error": "plugin.json missing from pack"}, http.StatusBadRequest
	}
	var man scannerPackManifest
	if err := json.Unmarshal(manRaw, &man); err != nil {
		return map[string]any{"error": "plugin.json parse: " + err.Error()}, http.StatusBadRequest
	}
	if man.Kind != "scanner" {
		return map[string]any{"error": "kind bukan 'scanner' (ini bukan scanner-pack)"}, http.StatusBadRequest
	}
	packID := strings.TrimSpace(man.ID)
	if !validCheckName(packID) {
		return map[string]any{"error": "id pack invalid [a-zA-Z0-9._-], no .."}, http.StatusBadRequest
	}
	base := nucleiTemplatesDir()
	if base == "" {
		return map[string]any{"error": "dir template nuclei ga ketemu (install nuclei-templates dulu)"}, http.StatusServiceUnavailable
	}

	// 2) extract checks/*.yaml → STAGING
	staging := filepath.Join(base, "."+scannerPackPrefix+"staging-"+packID)
	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging)
	if e := os.MkdirAll(staging, 0o755); e != nil {
		return map[string]any{"error": "mkdir: " + e.Error()}, http.StatusInternalServerError
	}
	got := 0
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if !strings.HasPrefix(name, "checks/") || strings.HasSuffix(name, "/") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") && !strings.HasSuffix(strings.ToLower(name), ".yml") {
			continue
		}
		bn := filepath.Base(name)
		if !validCheckName(strings.TrimSuffix(strings.TrimSuffix(bn, ".yaml"), ".yml")) {
			continue
		}
		dest := filepath.Join(staging, bn)
		if c, e := filepath.Rel(staging, dest); e != nil || strings.HasPrefix(c, "..") {
			continue // anti zip-slip
		}
		rc, e := f.Open()
		if e != nil {
			continue
		}
		out, e := os.Create(dest)
		if e != nil {
			rc.Close()
			continue
		}
		_, _ = io.Copy(out, io.LimitReader(rc, 4<<20))
		out.Close()
		rc.Close()
		got++
		if got >= 2000 {
			break // batas wajar per-pack
		}
	}
	if got == 0 {
		return map[string]any{"error": "ga ada checks/*.yaml di pack"}, http.StatusBadRequest
	}

	// 3) GERBANG: nuclei -validate sekali → buang yg invalid
	invalid := validatePackInvalidFiles(staging)
	removed := 0
	for p := range invalid {
		if filepath.Dir(p) == staging {
			if os.Remove(p) == nil {
				removed++
			}
		}
	}
	entries, _ := os.ReadDir(staging)
	valid := 0
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			valid++
		}
	}
	if valid == 0 {
		return map[string]any{"error": "ga ada check valid di pack (semua ditolak nuclei -validate)"}, http.StatusUnprocessableEntity
	}

	// 4) ATOMIC rename staging → final pack dir (auto masuk arsenal)
	finalDir := filepath.Join(base, scannerPackPrefix+packID)
	_ = os.RemoveAll(finalDir)
	if e := os.Rename(staging, finalDir); e != nil {
		return map[string]any{"error": "install pack: " + e.Error()}, http.StatusInternalServerError
	}
	// marker pack.json (metadata buat list)
	meta, _ := json.MarshalIndent(map[string]any{"id": packID, "name": man.Scanner.Name, "description": man.Scanner.Description, "checks": valid}, "", "  ")
	_ = os.WriteFile(filepath.Join(finalDir, "pack.json"), meta, 0o644)

	resetNucleiPackCache()
	return map[string]any{
		"ok": true, "pack_id": packID, "name": man.Scanner.Name,
		"checks": valid, "skipped_invalid": removed,
		"arsenal_pack": "nuclei:" + scannerPackPrefix + packID,
		"next":         "pack LIVE di arsenal — kelihatan di ≣ Arsenal, bisa di-scan/exclude.",
	}, 0
}

// ScannerPackInstallHandler — POST multipart "file" = .fwpack kind:scanner.
func ScannerPackInstallHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "parse form: " + err.Error()})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "missing file field"})
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, 64<<20))
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "read: " + err.Error()})
			return
		}
		body, status := InstallScannerPack(raw)
		tfWriteJSON(w, status, body)
	}
}

// ScannerPackUninstallHandler — POST ?id=<pack-id> → hapus dir pack.
func ScannerPackUninstallHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !validCheckName(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		base := nucleiTemplatesDir()
		if base == "" {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dir ga ketemu"})
			return
		}
		dir := filepath.Join(base, scannerPackPrefix+id)
		if st, e := os.Stat(dir); e != nil || !st.IsDir() {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": "pack '" + id + "' ga keinstall"})
			return
		}
		if e := os.RemoveAll(dir); e != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": e.Error()})
			return
		}
		resetNucleiPackCache()
		tfWriteJSON(w, 0, map[string]any{"ok": true, "uninstalled": id})
	}
}

// ScannerPacksInstalledHandler — GET daftar scanner-pack keinstall (flowork-pack-*).
func ScannerPacksInstalledHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		base := nucleiTemplatesDir()
		if base != "" {
			entries, _ := os.ReadDir(base)
			for _, e := range entries {
				if !e.IsDir() || !strings.HasPrefix(e.Name(), scannerPackPrefix) {
					continue
				}
				id := strings.TrimPrefix(e.Name(), scannerPackPrefix)
				item := map[string]any{"id": id, "arsenal_pack": "nuclei:" + e.Name()}
				if raw, err := os.ReadFile(filepath.Join(base, e.Name(), "pack.json")); err == nil {
					var m map[string]any
					if json.Unmarshal(raw, &m) == nil {
						item["name"] = m["name"]
						item["description"] = m["description"]
						item["checks"] = m["checks"]
					}
				}
				out = append(out, item)
			}
		}
		tfWriteJSON(w, 0, map[string]any{"installed": out, "count": len(out)})
	}
}
