// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

		fmt.Fprintf(w, "\n[sneakernet export error: %v]\n", err)
		return
	}
}

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

	if err := r.ParseMultipartForm(200 << 20); err != nil {
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

	existingHash := sneakernet.FingerprintExisting(targetRoot)
	res, err := sneakernet.Import(file, sneakernet.ImportOptions{
		TargetRoot: targetRoot,
		Passphrase: passphrase,
	})
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}

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
		"idempotent":    idempotent,
		"boot_ready":    bootReady,
		"verify_error":  errString(vErr),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func persistManifest(targetRoot string, m sneakernet.Manifest) {
	dir := filepath.Join(targetRoot, "_meta")
	_ = os.MkdirAll(dir, 0o755)
	raw, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644)
}
