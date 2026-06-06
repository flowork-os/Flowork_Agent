// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 19 phase 1 endpoints — export download + import upload.
//   Passphrase via HTTP header X-Sneakernet-Passphrase (anti-log query
//   string). Phase 2 (multi-file batch, resumable upload) → tambah file
//   baru, JANGAN modify ini.
//
// sneakernet.go — Section 19 phase 1: HTTP endpoints.

package agentmgr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/sneakernet"
)

// SneakernetExportHandler — POST /api/agents/sneakernet/export?id=<agent>
// Header X-Sneakernet-Passphrase: <passphrase> (optional — kalau ada,
// AES-256-GCM encrypt). Response: octet-stream `.fwsync` body.
func SneakernetExportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(agentID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid agent id"})
		return
	}
	passphrase := r.Header.Get("X-Sneakernet-Passphrase")
	folder := agentFolder(agentID)
	// Read agent version from manifest (best-effort).
	version := ""
	hostname, _ := os.Hostname()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s.fwsync"`, agentID))

	err := sneakernet.Export(w, sneakernet.ExportOptions{
		AgentID:    agentID,
		AgentRoot:  folder,
		Version:    version,
		HostOrigin: hostname,
		Passphrase: passphrase,
	})
	if err != nil {
		// Header sudah ke-set Content-Type binary; trailing error harus
		// inline (HTTP/1 ngga support late JSON kalau body udah dimulai).
		fmt.Fprintf(w, "\n[sneakernet export error: %v]\n", err)
		return
	}
}

// SneakernetImportHandler — POST /api/agents/sneakernet/import?target_id=<agent>
// Body: multipart/form-data field `file` berisi .fwsync.
// Header X-Sneakernet-Passphrase: <passphrase> (kalau encrypted).
//
// Target folder = agentFolder(target_id) — caller (Mr.Dev) konfirmasi target
// kosong / boleh overwrite.
func SneakernetImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	targetID := strings.TrimSpace(r.URL.Query().Get("target_id"))
	if !reID.MatchString(targetID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid target_id"})
		return
	}
	passphrase := r.Header.Get("X-Sneakernet-Passphrase")

	if err := r.ParseMultipartForm(200 << 20); err != nil { // 200MB cap
		httpx.WriteJSON(w, map[string]any{"error": "parse multipart: " + err.Error()})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "file field required: " + err.Error()})
		return
	}
	defer file.Close()

	targetRoot := agentFolder(targetID)
	// Section 19 phase 2: idempotency pre-fingerprint dari manifest existing.
	existingHash := sneakernet.FingerprintExisting(targetRoot)
	res, err := sneakernet.Import(file, sneakernet.ImportOptions{
		TargetRoot: targetRoot,
		Passphrase: passphrase,
	})
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	// Section 19 phase 2: Import doesn't persist manifest to disk —
	// write it for verify probe + future idempotency checks.
	persistManifest(targetRoot, res.Manifest)
	verified, vErr := sneakernet.VerifyImported(targetRoot)
	newHash := sneakernet.FingerprintManifest(verified)
	idempotent := existingHash != "" && existingHash == newHash
	bootReady := vErr == nil

	httpx.WriteJSON(w, map[string]any{
		"ok":            true,
		"target_id":     targetID,
		"target_root":   targetRoot,
		"manifest":      res.Manifest,
		"files_count":   res.FilesCount,
		"bytes_written": res.BytesWriten,
		"idempotent":    idempotent, // true kalau import sama file 2x
		"boot_ready":    bootReady,  // true kalau manifest valid + format_version OK
		"verify_error":  errString(vErr),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// persistManifest — Section 19 phase 2: write Manifest ke `_meta/manifest.json`
// supaya VerifyImported + FingerprintExisting bisa read post-import.
// Locked import.go ngga write to disk — handler wajib.
func persistManifest(targetRoot string, m sneakernet.Manifest) {
	dir := filepath.Join(targetRoot, "_meta")
	_ = os.MkdirAll(dir, 0o755)
	raw, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644)
}
