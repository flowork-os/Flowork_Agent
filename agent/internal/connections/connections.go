// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/loket"
)

var (
	errInvalidID = errors.New("invalid connector id")
	errNotFound  = errors.New("connector not found")
)

var secretKeyRe = regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key|\bkey\b)`)

var connIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

const disabledMarker = ".connector-disabled"

const fwagentSuffix = ".fwagent"

type Connector struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Kind    string              `json:"kind"`
	Version string              `json:"version"`
	Enabled bool                `json:"enabled"`
	Config  []loket.ConfigField `json:"config,omitempty"`
	Values  map[string]string   `json:"values,omitempty"`
}

func folder(id string) (string, bool) {
	if !connIDRe.MatchString(id) {
		return "", false
	}
	return filepath.Join(loader.AgentsDir(), id+fwagentSuffix), true
}

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

func IsEnabled(id string) bool {
	if isNative(id) {
		return true
	}
	dir, ok := folder(id)
	if !ok {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, disabledMarker)); err == nil {
		return false
	}
	return true
}

func List() []Connector {
	out := nativeList()
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
			continue
		}
		values, _ := GetConfigMasked(id)
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
	if err != nil || secrets == nil {
		secrets = map[string]string{}
	}

	if fdb, ferr := floworkdb.Shared(); ferr == nil {
		for _, f := range schemaOf(id) {
			if isSecretField(f) {
				if v, e := fdb.GetSecret(f.Key); e == nil && v != "" {
					secrets[f.Key] = v
				}
			}
		}
	}
	return secrets, nil
}

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

func SetConfig(id string, patch map[string]string) error {
	if isNative(id) {
		return nativeSetConfig(id, patch)
	}

	secretKey := map[string]bool{}
	for _, f := range schemaOf(id) {
		if isSecretField(f) {
			secretKey[f.Key] = true
		}
	}
	fdb, ferr := floworkdb.Shared()
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

		if secretKey[k] && ferr == nil {
			if v == "" {
				_ = fdb.DeleteSecret(k)
			} else if e := fdb.SetSecret(k, v); e != nil {
				return e
			}
			delete(secrets, k)
			continue
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

var configKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

func maskSecret(v string) string {
	if len(v) <= 4 {
		return "••••"
	}
	return "••••••" + v[len(v)-4:]
}

func ownerCaps(consumes []string) []string {
	var bad []string
	for _, c := range consumes {
		if spec, ok := loket.LookupCap(c); ok && spec.Grant == loket.GrantOwner {
			bad = append(bad, c)
		}
	}
	return bad
}

const maxPackFiles = 200

type channelPackManifest struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Channel struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"channel"`
}

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
			continue
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

	sm := readManifest(staging)
	if sm == nil || sm.Kind != loket.KindChannel {
		return map[string]any{"error": "extracted manifest is not kind:channel"}, http.StatusBadRequest
	}
	if sm.ID != id {
		return map[string]any{"error": "manifest id mismatch with pack id"}, http.StatusBadRequest
	}

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
