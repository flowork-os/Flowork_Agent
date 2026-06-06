// pack_extract.go — helper SHARED extract wasm-agent dari .fwpack (dedupe tool+slash).
//
// Extract `agents/<id>/` → STAGING → tulis marker → ATOMIC rename ke
// `<AgentsDir>/<id>.fwagent` (hot-load fsnotify, 1 dir lengkap). Anti zip-slip.
// Balik (errBody, status); status 0 = ok (errBody nil). Logika ini DULU di-copy
// identik di tool_install.go + slash_install.go — sekarang 1 sumber.

package main

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/kernel/loader"
)

func extractWasmAgentPack(zr *zip.Reader, agentID, stagingPrefix, markerName string, markerData []byte) (map[string]any, int) {
	agentsRoot := loader.AgentsDir()
	staging := filepath.Join(agentsRoot, stagingPrefix+agentID)
	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging)
	got := 0
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		prefix := "agents/" + agentID + "/"
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
	}
	if got == 0 {
		return map[string]any{"error": "agent.wasm ga ketemu di pack (agents/" + agentID + "/)"}, http.StatusBadRequest
	}
	// marker (tool.json / slash.json) di staging → ikut ke-rename atomik.
	_ = os.WriteFile(filepath.Join(staging, markerName), markerData, 0o644)

	finalDir := filepath.Join(agentsRoot, agentID+".fwagent")
	_ = os.RemoveAll(finalDir)
	if e := os.Rename(staging, finalDir); e != nil {
		return map[string]any{"error": "install agent-pack: " + e.Error()}, http.StatusInternalServerError
	}
	return nil, 0
}
