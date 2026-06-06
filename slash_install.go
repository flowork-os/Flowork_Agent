// slash_install.go — install/uninstall/list SLASH-PACK plug-and-play.
//
//	POST /api/slash/install   (multipart "file" = .fwpack, kind:slash)
//	POST /api/slash/uninstall?name=<cmd>
//	GET  /api/slash/installed
//
// .fwpack slash layout: plugin.json {kind:"slash", slash:{...spec, agent_id}} +
// agents/<agent_id>/{agent.wasm, manifest.json}. REUSE pola extract tool/task.

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/slashcmd"
)

type slashPackManifest struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	Slash slashSpec `json:"slash"`
}

func installSlashPack(host *kernelhost.Host, raw []byte) (map[string]any, int) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return map[string]any{"error": "not a valid zip: " + err.Error()}, http.StatusBadRequest
	}
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
	var man slashPackManifest
	if err := json.Unmarshal(manRaw, &man); err != nil {
		return map[string]any{"error": "plugin.json parse: " + err.Error()}, http.StatusBadRequest
	}
	if man.Kind != "slash" {
		return map[string]any{"error": "kind bukan 'slash' (ini bukan slash-pack)"}, http.StatusBadRequest
	}
	spec := man.Slash
	spec.Name = strings.ToLower(strings.TrimSpace(spec.Name))
	if !slashNameRe.MatchString(spec.Name) {
		return map[string]any{"error": "slash.name invalid (^[a-z][a-z0-9_-]{1,31}$)"}, http.StatusBadRequest
	}
	if !pluginIDRe.MatchString(spec.AgentID) {
		return map[string]any{"error": "slash.agent_id invalid"}, http.StatusBadRequest
	}
	// proteksi: nama/alias udah ada (builtin atau slash lain) → tolak (uninstall dulu).
	if slashcmd.Has(spec.Name) {
		return map[string]any{"error": "slash /" + spec.Name + " udah ada (builtin/terinstall) — uninstall dulu"}, http.StatusConflict
	}
	for _, a := range spec.Aliases {
		if slashcmd.Has(strings.ToLower(a)) {
			return map[string]any{"error": "alias /" + a + " bentrok — ganti/ilangin"}, http.StatusConflict
		}
	}
	// SECURITY: same as tool packs — the kind-dispatch path skips the agent
	// caps-consent gate, so refuse dangerous caps (exec:/secret:/fs:shared/...) here.
	if danger := scanPackCaps(zr); len(danger) > 0 {
		return map[string]any{
			"error":          "slash pack requests dangerous capabilities — refused",
			"dangerous_caps": danger,
		}, http.StatusForbidden
	}

	// extract wasm → staging → atomic rename (hot-load). [shared: pack_extract.go]
	markerRaw, _ := json.MarshalIndent(spec, "", "  ")
	if eb, st := extractWasmAgentPack(zr, spec.AgentID, ".slashpack-staging-", "slash.json", markerRaw); st != 0 {
		return eb, st
	}
	smoke := smokeTestSynth(host, spec.AgentID)
	if err := registerWasmSlash(host, spec, false); err != nil {
		return map[string]any{"error": "register slash: " + err.Error()}, http.StatusInternalServerError
	}
	return map[string]any{
		"ok": true, "slash": spec.Name, "aliases": spec.Aliases, "agent_id": spec.AgentID,
		"smoke": smoke, "next": "slash LIVE — pakai /" + spec.Name + " <args>.",
	}, 0
}

func slashInstallHandler(host *kernelhost.Host) http.HandlerFunc {
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
		raw, err := io.ReadAll(io.LimitReader(file, 128<<20))
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "read: " + err.Error()})
			return
		}
		body, status := installSlashPack(host, raw)
		tfWriteJSON(w, status, body)
	}
}

func slashUninstallHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
		if name == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
			return
		}
		// proteksi: cuma slash PLUGIN (punya marker) yang boleh dicabut.
		agentID := findSlashAgent(name)
		if agentID == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "slash /" + name + " bukan plugin (atau ga ada) — ga bisa di-uninstall"})
			return
		}
		slashcmd.Unregister(name)
		_ = os.RemoveAll(filepath.Join(loader.AgentsDir(), agentID+".fwagent"))
		tfWriteJSON(w, 0, map[string]any{"ok": true, "uninstalled": name, "agent_removed": agentID})
	}
}

func slashInstalledHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, s := range installedSlashPacks() {
			out = append(out, map[string]any{
				"name": s.Name, "description": s.Description, "aliases": s.Aliases,
			})
		}
		tfWriteJSON(w, 0, map[string]any{"installed": out, "count": len(out)})
	}
}
