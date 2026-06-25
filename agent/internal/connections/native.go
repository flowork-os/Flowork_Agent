// native.go — built-in HOST-SIDE connectors (CLI, MCP).
//
// A CLI and an MCP server can't run inside wasm (no terminal / no stdio there), so
// — exactly as the Connections design says — they are host-side binaries shipped
// with the engine. They are still connectors: dumb pipes that forward a message to
// an agent and default to mr-flow. Unlike a wasm connector they have no .fwagent
// folder, so they keep their self-managed config in their OWN folder under
// ~/.flowork/connectors/<id>/config.json — the very file the cli/mcp binaries read.
// This is how the "one roof" gallery shows them beside the wasm connectors.
package connections

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/loket"
)

// defaultNativeAgent — default "Target agent" connector native (CLI/MCP). SATU switch
// dgn host: ENV FLOWORK_ORCHESTRATOR, default mr-flow (orchestrator LIVE; mr-flow-next
// belum ke-deploy). Lihat lock/mrflow.md §6b.
func defaultNativeAgent() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ORCHESTRATOR")); v != "" {
		return v
	}
	return "mr-flow"
}

// nativeDefs is the built-in connector list. Always present, always enabled (a
// binary can't be "off"); they cannot be uninstalled — only configured.
var nativeDefs = []Connector{
	{ID: "cli", Name: "CLI (terminal)", Kind: "native", Config: nativeCLISchema},
	{ID: "mcp", Name: "MCP Server", Kind: "native", Config: nativeMCPSchema},
}

// The schema keys are the EXACT keys the cli/mcp binaries read from config.json.
var nativeCLISchema = []loket.ConfigField{
	{Key: "agent", Label: "Target agent", Type: "text", Default: defaultNativeAgent(), Help: "which agent the CLI talks to"},
	{Key: "base", Label: "Flowork URL", Type: "text", Default: "http://127.0.0.1:1987"},
}

var nativeMCPSchema = []loket.ConfigField{
	{Key: "agent", Label: "Target agent", Type: "text", Default: defaultNativeAgent(), Help: "which agent MCP clients chat with"},
}

func isNative(id string) bool {
	for _, n := range nativeDefs {
		if n.ID == id {
			return true
		}
	}
	return false
}

func nativeSchema(id string) []loket.ConfigField {
	for _, n := range nativeDefs {
		if n.ID == id {
			return n.Config
		}
	}
	return nil
}

// nativeDir is a native connector's own config folder (multi-OS, filepath only).
// The cli/mcp binaries read their config.json from exactly here.
func nativeDir(id string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".flowork", "connectors", id)
}

func nativeConfigPath(id string) string { return filepath.Join(nativeDir(id), "config.json") }

func nativeGetConfig(id string) map[string]string {
	out := map[string]string{}
	if raw, err := os.ReadFile(nativeConfigPath(id)); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func nativeSetConfig(id string, patch map[string]string) error {
	cfg := nativeGetConfig(id)
	for k, v := range patch {
		if !configKeyRe.MatchString(k) {
			return errors.New("invalid config key " + k)
		}
		if v == "" {
			delete(cfg, k)
		} else {
			cfg[k] = v
		}
	}
	if err := os.MkdirAll(nativeDir(id), 0o755); err != nil {
		return err
	}
	blob, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := nativeConfigPath(id) + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, nativeConfigPath(id))
}

// nativeList returns the built-in connectors with their current (default-filled,
// masked) values for the gallery.
func nativeList() []Connector {
	out := make([]Connector, 0, len(nativeDefs))
	for _, n := range nativeDefs {
		c := n
		c.Enabled = true // a host-side binary is always available
		c.Values = nativeMaskedValues(n.ID)
		out = append(out, c)
	}
	return out
}

func nativeMaskedValues(id string) map[string]string {
	cur := nativeGetConfig(id)
	out := map[string]string{}
	for _, f := range nativeSchema(id) {
		v := cur[f.Key]
		if v == "" {
			v = f.Default
		}
		if v != "" && (f.Type == "secret" || secretKeyRe.MatchString(f.Key)) {
			v = maskSecret(v)
		}
		out[f.Key] = v
	}
	return out
}
