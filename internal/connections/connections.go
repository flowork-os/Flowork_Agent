// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: Connections registry core (ROADMAP_CONNECTIONS.md). The connector lifecycle
//
//   - the kind:channel install gerbang. Adding a connector = dropping a folder, never
//     editing this file. The security gate (refuse GrantOwner caps), id-validation and
//     zip-slip guard here are load-bearing — don't weaken them.
//     2026-06-06 (owner-directed): config now reads/writes the connector's OWN agentdb
//     store secrets (Load→merge→Save) — the home for a connector's CHAT credential that
//     buildAgentEnv injects. Manifest-declared fields (loket.ConfigField) so the GUI
//     hardcodes no connector's keys. NOTE: owner NOTIFICATIONS are a separate concern —
//     they live in Settings → Notifications (floworkdb), not in any connector.
//
// Package connections is the isolated registry for "Connections" — Flowork's
// universal connector system (telegram/discord/email/cli/schedule/mcp). A
// connector is a kind:channel wasm module living in its OWN .fwagent folder; it
// bridges an external surface to the loket bus and owns nothing else.
//
// WHY this package is its own thing (read ROADMAP_CONNECTIONS.md for the full
// rationale): the headline guarantee is "1 connector errors → fix ONLY its
// folder." That is why:
//   - every connector is a self-contained folder (wasm + manifest + its own state);
//   - enabled-state is a MARKER FILE inside the connector folder, never a shared
//     table — a central row could couple connectors and break isolation;
//   - uninstall is just removing the folder: no central cleanup, nothing dangling;
//   - this management logic lives in its OWN package, so a bug here can't reach
//     agentmgr/main (same pattern as internal/scanapi for scanner packs).
//
// Multi-OS: pure-Go, paths via filepath only — no hardcoded separators.
package connections

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/loket"
)

var (
	errInvalidID = errors.New("invalid connector id")
	errNotFound  = errors.New("connector not found")
)

// secretKeyRe flags config keys whose VALUE is a credential, so the management API
// can mask them. The value still lives in the connector's own folder (the owner
// chose self-managed tokens); masking only keeps it out of list/GET responses.
var secretKeyRe = regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key|\bkey\b)`)

// connIDRe is the allowed connector id shape. Lowercase + digits + dash/underscore,
// no dots or slashes — so an id can never become a path-traversal segment when it is
// turned into a folder name. Matches the engine's pluginIDRe.
var connIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

// disabledMarker is the file that, when present in a connector's folder, marks it
// disabled. Absence = enabled. State-in-folder = isolation (see package doc).
const disabledMarker = ".connector-disabled"

// fwagentSuffix is the folder suffix every module (and thus every connector) uses.
const fwagentSuffix = ".fwagent"

// Connector is the management view of one installed connector.
type Connector struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Kind    string              `json:"kind"`
	Version string              `json:"version"`
	Enabled bool                `json:"enabled"`
	Config  []loket.ConfigField `json:"config,omitempty"` // declared settable fields (schema)
	Values  map[string]string   `json:"values,omitempty"` // current values (secrets masked)
}

// folder returns the connector's own folder, validating the id first so a crafted
// id can never escape AgentsDir.
func folder(id string) (string, bool) {
	if !connIDRe.MatchString(id) {
		return "", false
	}
	return filepath.Join(loader.AgentsDir(), id+fwagentSuffix), true
}

// readManifest reads a module folder's loket.json (fallback manifest.json) and
// returns the parsed manifest, or nil if it isn't a valid manifest.
func readManifest(dir string) *loket.Manifest {
	for _, name := range []string{"loket.json", "manifest.json"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if m, err := loket.ParseManifest(raw); err == nil {
			return m
		}
	}
	return nil
}

// IsEnabled reports whether a connector is enabled (no disabled-marker present).
// Unknown/invalid id → false.
func IsEnabled(id string) bool {
	if isNative(id) {
		return true // a host-side binary is always available
	}
	dir, ok := folder(id)
	if !ok {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, disabledMarker)); err == nil {
		return false // marker present → disabled
	}
	return true
}

// List enumerates every connector: the built-in host-side ones (CLI, MCP) plus
// every installed kind:channel wasm module under AgentsDir — one roof.
func List() []Connector {
	out := nativeList() // built-in CLI + MCP first
	entries, err := os.ReadDir(loader.AgentsDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), fwagentSuffix) {
			continue
		}
		id := strings.TrimSuffix(e.Name(), fwagentSuffix)
		if !connIDRe.MatchString(id) {
			continue
		}
		dir := filepath.Join(loader.AgentsDir(), e.Name())
		m := readManifest(dir)
		if m == nil || m.Kind != loket.KindChannel {
			continue // only channel-kind modules are connectors
		}
		values, _ := GetConfigMasked(id) // current settings (secrets masked)
		out = append(out, Connector{
			ID:      id,
			Name:    m.Name,
			Kind:    string(m.Kind),
			Version: m.Version,
			Enabled: IsEnabled(id),
			Config:  m.Config,
			Values:  values,
		})
	}
	return out
}

// SetEnabled toggles a connector on/off by adding/removing its disabled-marker.
// The connector daemon consults IsEnabled to decide whether to go live.
func SetEnabled(id string, enabled bool) error {
	if isNative(id) {
		return errors.New("built-in connector is always on")
	}
	dir, ok := folder(id)
	if !ok {
		return errInvalidID
	}
	if _, err := os.Stat(dir); err != nil {
		return errNotFound
	}
	marker := filepath.Join(dir, disabledMarker)
	if enabled {
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(marker, []byte("disabled by owner\n"), 0o644)
}

// Uninstall removes a connector entirely — just its folder. Nothing central to
// clean up, because nothing about the connector lived outside its folder.
func Uninstall(id string) error {
	if isNative(id) {
		return errors.New("built-in connector can't be uninstalled")
	}
	dir, ok := folder(id)
	if !ok {
		return errInvalidID
	}
	if _, err := os.Stat(dir); err != nil {
		return errNotFound
	}
	return os.RemoveAll(dir)
}

// connectorStore opens a connector's OWN agentdb store — the single home for ITS
// config + credentials (its chat token, target agent, …). buildAgentEnv injects
// these secrets to the live connector at boot, so writing here reaches the connector
// without any copy in the agent. NOTE: this is the connector's CHAT credential and is
// SEPARATE from owner notifications, which live in Settings → Notifications (floworkdb).
func connectorStore(id string) (*agentdb.Store, error) {
	dir, ok := folder(id)
	if !ok {
		return nil, errInvalidID
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, errNotFound
	}
	return agentdb.Open(agentdb.Resolve(id, dir))
}

// schemaOf returns the config fields a connector declares in its manifest. The GUI
// renders these; the kernel hardcodes no connector's keys (plug-and-play).
func schemaOf(id string) []loket.ConfigField {
	if isNative(id) {
		return nativeSchema(id)
	}
	dir, ok := folder(id)
	if !ok {
		return nil
	}
	if m := readManifest(dir); m != nil {
		return m.Config
	}
	return nil
}

// GetConfig returns a connector's stored values (its own store secrets), keyed as
// the connector reads them. Real values — internal use only.
func GetConfig(id string) (map[string]string, error) {
	if isNative(id) {
		return nativeGetConfig(id), nil
	}
	st, err := connectorStore(id)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	secrets, err := st.Secrets()
	if err != nil {
		return map[string]string{}, nil
	}
	return secrets, nil
}

// GetConfigMasked returns only the declared schema fields, secret-typed values
// masked — for the management API, so a token never rides a response to the browser.
func GetConfigMasked(id string) (map[string]string, error) {
	cur, err := GetConfig(id)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range schemaOf(id) {
		v := cur[f.Key]
		if v != "" && (f.Type == "secret" || secretKeyRe.MatchString(f.Key)) {
			v = maskSecret(v)
		}
		out[f.Key] = v
	}
	return out, nil
}

// SetConfig merges patch into the connector's OWN store secrets — the single source.
// Load→merge→Save preserves the other secrets (store.Save full-replaces the secrets
// table, so a blind partial write would wipe them). Empty value deletes a key.
func SetConfig(id string, patch map[string]string) error {
	if isNative(id) {
		return nativeSetConfig(id, patch) // built-in: config in its own ~/.flowork/connectors/<id>/
	}
	st, err := connectorStore(id)
	if err != nil {
		return err
	}
	defer st.Close()
	cfg, err := st.Load()
	if err != nil {
		return err
	}
	secrets, _ := cfg["secrets"].(map[string]any)
	if secrets == nil {
		secrets = map[string]any{}
	}
	for k, v := range patch {
		if !configKeyRe.MatchString(k) {
			return errors.New("invalid config key " + k)
		}
		if v == "" {
			delete(secrets, k)
		} else {
			secrets[k] = v
		}
	}
	cfg["secrets"] = secrets
	return st.Save(cfg)
}

// configKeyRe bounds config keys to safe identifiers (so a key can't be a path or
// shell-meta when later injected as an env var name).
var configKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// maskSecret shows only the last 4 chars of a credential.
func maskSecret(v string) string {
	if len(v) <= 4 {
		return "••••"
	}
	return "••••••" + v[len(v)-4:]
}

// ownerCaps returns the subset of consumed caps that are high-risk (GrantOwner) in
// the frozen loket vocabulary — the caps an install gerbang must gate.
func ownerCaps(consumes []string) []string {
	var bad []string
	for _, c := range consumes {
		if spec, ok := loket.LookupCap(c); ok && spec.Grant == loket.GrantOwner {
			bad = append(bad, c)
		}
	}
	return bad
}

// maxPackFiles bounds how many files a single connector pack may contain — a cheap
// guard against a zip with a runaway number of entries.
const maxPackFiles = 200

// channelPackManifest is the plugin.json shape for a kind:channel pack.
type channelPackManifest struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Channel struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"channel"`
}

// InstallChannelPack extracts a kind:channel .fwpack into its own connector folder
// (staging + atomic rename → fsnotify hot-load). The wasm payload lives at
// agents/<id>/* exactly like an agent pack. Returns (body, status); status 0 = ok.
//
// Mirrors scanapi.InstallScannerPack so the gerbang stays uniform across kinds.
func InstallChannelPack(raw []byte) (map[string]any, int) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return map[string]any{"error": "not a valid zip: " + err.Error()}, http.StatusBadRequest
	}
	var manRaw []byte
	for _, f := range zr.File {
		base := strings.TrimPrefix(f.Name, "./")
		if base == "plugin.json" || strings.HasSuffix(base, "/plugin.json") {
			if rc, e := f.Open(); e == nil {
				manRaw, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
				rc.Close()
			}
			break
		}
	}
	if manRaw == nil {
		return map[string]any{"error": "plugin.json missing from pack"}, http.StatusBadRequest
	}
	var man channelPackManifest
	if err := json.Unmarshal(manRaw, &man); err != nil {
		return map[string]any{"error": "plugin.json parse: " + err.Error()}, http.StatusBadRequest
	}
	if man.Kind != "channel" {
		return map[string]any{"error": "kind is not 'channel' (this is not a connector pack)"}, http.StatusBadRequest
	}
	id := strings.TrimSpace(man.ID)
	if !connIDRe.MatchString(id) {
		return map[string]any{"error": "connector id invalid (^[a-z0-9][a-z0-9_-]{1,63}$)"}, http.StatusBadRequest
	}

	agentsRoot := loader.AgentsDir()
	staging := filepath.Join(agentsRoot, ".connector-staging-"+id)
	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging)

	prefix := "agents/" + id + "/"
	got := 0
	var wasmSeen bool
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
		if strings.HasSuffix(strings.ToLower(rel), ".wasm") {
			wasmSeen = true
		}
		if got >= maxPackFiles {
			break
		}
	}
	if got == 0 || !wasmSeen {
		return map[string]any{"error": "no agent.wasm under agents/" + id + "/ in pack"}, http.StatusBadRequest
	}
	// The extracted manifest MUST itself declare kind:channel — a pack can't smuggle
	// in an agent of another kind under the connector gerbang.
	sm := readManifest(staging)
	if sm == nil || sm.Kind != loket.KindChannel {
		return map[string]any{"error": "extracted manifest is not kind:channel"}, http.StatusBadRequest
	}
	if sm.ID != id {
		return map[string]any{"error": "manifest id mismatch with pack id"}, http.StatusBadRequest
	}
	// SECURITY: the loket grants a module exactly the caps its manifest consumes, so
	// the INSTALL gerbang is where high-risk caps must be stopped. A connector is a
	// dumb pipe — it needs only the bus (host_net_fetch is a wasm host import, NOT a
	// loket cap). Refuse any GrantOwner cap (fs/exec/http.fetch); a third-party
	// connector has no business asking for them and would otherwise be auto-granted.
	if dangerous := ownerCaps(sm.Consumes); len(dangerous) > 0 {
		return map[string]any{
			"error":          "connector requests high-risk capabilities — refused",
			"dangerous_caps": dangerous,
			"hint":           "a connector should consume only bus.* / store.* — it polls via host fetch, not fs/exec/http",
		}, http.StatusForbidden
	}

	finalDir := filepath.Join(agentsRoot, id+fwagentSuffix)
	_ = os.RemoveAll(finalDir)
	if e := os.Rename(staging, finalDir); e != nil {
		return map[string]any{"error": "install connector: " + e.Error()}, http.StatusInternalServerError
	}
	return map[string]any{
		"ok":        true,
		"connector": id,
		"name":      sm.Name,
		"files":     got,
		"next":      "connector LIVE (hot-load) — set token + enable in Connections.",
	}, 0
}
