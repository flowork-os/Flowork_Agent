// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-12
// Reason: Centralize connector SECRETS into Settings → API Keys (global floworkdb),
//   the single secret store. GlobalSecretEnvKeys feeds kernelhost.EnvForwardKeys
//   (the frozen plug-and-play hook) so a connector's token reaches it via env with
//   NO frozen edit; MigrateSchemaSecretsToGlobal relocates pre-existing per-agent
//   tokens once. Secret keys are derived from each connector's own schema — a new
//   connector is centralized automatically. Audited + build/test green.
//
// central.go — central (Settings-backed) secret storage for connectors.
package connections

import (
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/loket"
)

// connectorIDs returns every installed channel-kind connector id.
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

// isSecretField reports whether a connector schema field holds a secret.
func isSecretField(f loket.ConfigField) bool {
	return f.Type == "secret" || secretKeyRe.MatchString(f.Key)
}

// connectorSecretKeys returns the secret-typed env keys a connector declares.
func connectorSecretKeys(id string) []string {
	var out []string
	for _, f := range schemaOf(id) {
		if isSecretField(f) {
			out = append(out, f.Key)
		}
	}
	return out
}

// GlobalSecretEnvKeys — the secret-typed env keys that THIS agent should receive
// from Settings → API Keys (global floworkdb). It returns a connector's OWN declared
// secret keys, and nothing for any other agent — so a bot token reaches only its
// connector, never an unrelated agent (which could double-poll the same bot). Wired
// into the per-agent kernelhost.EnvForwardKeys hook from main.go; a NEW connector
// with a secret field is handled automatically — no frozen-file edit.
func GlobalSecretEnvKeys(agentID string) []string {
	dir, ok := folder(agentID)
	if !ok {
		return nil
	}
	m := readManifest(dir)
	if m == nil || m.Kind != loket.KindChannel {
		return nil // not a connector → forward no connector secret to it
	}
	return connectorSecretKeys(agentID)
}

// MigrateSchemaSecretsToGlobal moves connector secrets that still sit in a per-agent
// store into the global Settings store, then drops the per-agent copy (so a stale
// copy can't shadow a Settings edit). Idempotent; never overwrites a value already
// set in Settings. Returns how many it moved. Call once at boot, BEFORE global
// secrets are injected into the process env.
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
