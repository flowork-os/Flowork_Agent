// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package connections

import (
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/loket"
)

func connectorIDs() []string {
	entries, err := os.ReadDir(loader.AgentsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), fwagentSuffix) {
			continue
		}
		id := strings.TrimSuffix(e.Name(), fwagentSuffix)
		if !connIDRe.MatchString(id) {
			continue
		}
		m := readManifest(filepath.Join(loader.AgentsDir(), e.Name()))
		if m == nil || m.Kind != loket.KindChannel {
			continue
		}
		out = append(out, id)
	}
	return out
}

func isSecretField(f loket.ConfigField) bool {
	return f.Type == "secret" || secretKeyRe.MatchString(f.Key)
}

func connectorSecretKeys(id string) []string {
	var out []string
	for _, f := range schemaOf(id) {
		if isSecretField(f) {
			out = append(out, f.Key)
		}
	}
	return out
}

func GlobalSecretEnvKeys(agentID string) []string {
	dir, ok := folder(agentID)
	if !ok {
		return nil
	}
	m := readManifest(dir)
	if m == nil || m.Kind != loket.KindChannel {
		return nil
	}
	return connectorSecretKeys(agentID)
}

func MigrateSchemaSecretsToGlobal() int {
	fdb, err := floworkdb.Shared()
	if err != nil {
		return 0
	}
	moved := 0
	for _, id := range connectorIDs() {
		keys := connectorSecretKeys(id)
		if len(keys) == 0 {
			continue
		}
		st, serr := connectorStore(id)
		if serr != nil {
			continue
		}
		cfg, lerr := st.Load()
		if lerr != nil {
			st.Close()
			continue
		}
		secrets, _ := cfg["secrets"].(map[string]any)
		changed := false
		for _, k := range keys {
			v, ok := secrets[k].(string)
			if !ok || strings.TrimSpace(v) == "" {
				continue
			}
			if cur, _ := fdb.GetSecret(k); strings.TrimSpace(cur) == "" {
				_ = fdb.SetSecret(k, v)
			}
			delete(secrets, k)
			changed = true
			moved++
		}
		if changed {
			cfg["secrets"] = secrets
			_ = st.Save(cfg)
		}
		st.Close()
	}
	return moved
}
