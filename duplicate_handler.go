// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked at: 2026-06-06
// Reason: Final pre-prod Threat Radar audit. nil-map critical at line 76 fixed
//   (pre-allocated map literal so a `null` manifest can't panic the writes).
//   Verified e2e: baseline scan critical 1→0. id validated (dupIDRe), staging→
//   atomic rename, no secrets/brain copied.
//
// duplicate_handler.go — POST /api/agents/duplicate?id=<src>&new_id=<dst>
//
// Copy an existing agent into a new one: same wasm + manifest (id/name rewritten)
// + its config persona/tools/skills (NOT its secrets, NOT its brain — each agent
// owns its own memory). This is the "copas" recipe as one click: duplicate, then
// open the copy's Setting to change the persona. Staging → atomic rename so the
// hot-reload watcher loads the new agent cleanly.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/kernel/loader"
)

var dupIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

func agentDuplicateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	srcID := r.URL.Query().Get("id")
	newID := r.URL.Query().Get("new_id")
	if !dupIDRe.MatchString(srcID) || !dupIDRe.MatchString(newID) {
		tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id (^[a-z][a-z0-9-]{1,63}$)"})
		return
	}
	if srcID == newID {
		tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "new_id must differ from source"})
		return
	}
	agentsDir := loader.AgentsDir()
	srcDir := filepath.Join(agentsDir, srcID+".fwagent")
	dstDir := filepath.Join(agentsDir, newID+".fwagent")
	if _, err := os.Stat(srcDir); err != nil {
		tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": "source agent not installed: " + srcID})
		return
	}
	if _, err := os.Stat(dstDir); err == nil {
		tfWriteJSON(w, http.StatusConflict, map[string]any{"error": "target id already exists: " + newID})
		return
	}

	staging := filepath.Join(agentsDir, ".dup-staging-"+newID)
	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging)
	if err := os.MkdirAll(filepath.Join(staging, "workspace"), 0o755); err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "mkdir: " + err.Error()})
		return
	}

	// 1) wasm
	if err := copyFileDup(filepath.Join(srcDir, "agent.wasm"), filepath.Join(staging, "agent.wasm")); err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "copy wasm: " + err.Error()})
		return
	}

	// 2) manifest — rewrite id + display_name, keep caps/entry/etc.
	manRaw, err := os.ReadFile(filepath.Join(srcDir, "manifest.json"))
	if err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "read manifest: " + err.Error()})
		return
	}
	// Pre-allocate with a map literal instead of a bare nil declaration:
	// unmarshaling the JSON token `null` into a nil map leaves it nil AND returns
	// no error → the writes below would panic ("assignment to entry in nil map").
	// A pre-allocated map stays non-nil for any input (null/object/empty), so the
	// writes are always safe.
	man := map[string]any{}
	if err := json.Unmarshal(manRaw, &man); err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "parse manifest: " + err.Error()})
		return
	}
	man["id"] = newID
	if dn, _ := man["display_name"].(string); dn != "" {
		man["display_name"] = dn + " (copy)"
	} else {
		man["display_name"] = newID
	}
	newMan, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), newMan, 0o644); err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "write manifest: " + err.Error()})
		return
	}

	// 3) config — copy persona/router/tools/skills ONLY. No secrets, no brain.
	srcDB := agentdb.Resolve(srcID, srcDir)
	if st, oerr := agentdb.Open(srcDB); oerr == nil {
		cfg, lerr := st.Load()
		st.Close()
		if lerr == nil {
			clean := map[string]any{}
			for _, key := range []string{"prompt", "router", "tools", "skills"} {
				if v, ok := cfg[key]; ok {
					clean[key] = v
				}
			}
			if len(clean) > 0 {
				dstDB := filepath.Join(staging, "workspace", "state.db")
				if dst, derr := agentdb.Open(dstDB); derr == nil {
					_ = dst.Save(clean)
					dst.Close()
				}
			}
		}
	}

	// 3b) persona + doctrine files travel with the copy too, if the agent keeps
	// them as transparent files in its folder (prompt.md / doktrin.md). Best-effort.
	srcWS := filepath.Dir(agentdb.Resolve(srcID, srcDir))
	for _, f := range []string{"prompt.md", "doktrin.md"} {
		_ = copyFileDup(filepath.Join(srcWS, f), filepath.Join(staging, "workspace", f))
	}

	// 4) atomic install → hot-reload watcher loads it.
	if err := os.Rename(staging, dstDir); err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "install: " + err.Error()})
		return
	}
	tfWriteJSON(w, 0, map[string]any{"ok": true, "new_id": newID, "source": srcID,
		"next": "open the new agent's Setting to change its persona/tools"})
}

func copyFileDup(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
