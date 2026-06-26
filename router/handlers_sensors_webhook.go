// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/ingest"
	"github.com/flowork-os/flowork_Router/internal/sensors"
)

const maxWebhookBodyBytes = 256 * 1024

func sensorsWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)

	sourceID := strings.TrimSpace(r.URL.Query().Get("source"))
	if sourceID == "" {
		http.Error(w, "source required", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-Sensor-Token"))
	if token == "" {
		http.Error(w, "X-Sensor-Token header required", http.StatusUnauthorized)
		return
	}
	if err := sensors.AuthSource(sourceID, token); err != nil {

		status := http.StatusUnauthorized
		if errors.Is(err, sensors.ErrInvalidSourceID) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	bodyBytes, rerr := io.ReadAll(r.Body)
	if rerr != nil {
		http.Error(w, "read body: "+rerr.Error(), http.StatusBadRequest)
		return
	}
	content := string(bodyBytes)
	if strings.TrimSpace(content) == "" {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	res := ingest.Submit(r.Context(), ingest.Req{
		Content:    content,
		Wing:       "webhook",
		Room:       sourceID,
		SourceType: "webhook",
		SourceFile: sourceID,
	})
	if res.Error != "" {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"algo_version": sensors.AlgoVersion,
		"drawer_id":    res.DrawerID,
		"added":        res.Added,
		"note":         res.Note,
	})
}

var _ = json.Marshal
