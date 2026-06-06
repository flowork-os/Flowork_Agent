package connections

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// buildChannelPack makes an in-memory .fwpack zip for a kind:channel connector.
// extraFiles maps zip-path -> content for adding edge-case entries (e.g. zip-slip).
func buildChannelPack(t *testing.T, id string, extraFiles map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	plugin, _ := json.Marshal(map[string]any{
		"id": id, "kind": "channel",
		"channel": map[string]any{"name": "Test Connector", "description": "x"},
	})
	add("plugin.json", string(plugin))
	manifest, _ := json.Marshal(map[string]any{
		"id": id, "kind": "channel", "name": "Test Connector",
		"version": "0.1.0", "abi_version": "1", "entry": "handle",
		"tier": "extension", "consumes": []string{"bus.request"},
		"config": []map[string]any{
			{"key": "BOT_TOKEN", "label": "Bot Token", "type": "secret"},
			{"key": "TARGET_AGENT", "label": "Target agent", "type": "text", "default": "mr-flow-next"},
		},
	})
	add("agents/"+id+"/loket.json", string(manifest))
	add("agents/"+id+"/agent.wasm", "\x00asm\x01\x00\x00\x00dummy")
	for name, content := range extraFiles {
		add(name, content)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestConnector_FullLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", root)
	const id = "test-conn"

	// install
	body, status := InstallChannelPack(buildChannelPack(t, id, nil))
	if status != 0 {
		t.Fatalf("install failed: %v", body)
	}
	if _, err := os.Stat(filepath.Join(root, id+".fwagent", "agent.wasm")); err != nil {
		t.Fatalf("wasm not extracted: %v", err)
	}

	// list shows it (alongside the built-in native connectors), enabled by default
	if c := findConn(id); c == nil || !c.Enabled {
		t.Fatalf("installed connector not listed/enabled: %+v", List())
	}
	// the built-in CLI + MCP connectors are always present in the gallery
	if findConn("cli") == nil || findConn("mcp") == nil {
		t.Errorf("native cli/mcp connectors missing from list")
	}

	// config: set a token — must land in the connector's OWN store (single source),
	// the same store buildAgentEnv reads at boot. No side file, nothing central.
	if err := SetConfig(id, map[string]string{"BOT_TOKEN": "secret-abcd1234", "TARGET_AGENT": "mr-flow-next"}); err != nil {
		t.Fatal(err)
	}
	if real, _ := GetConfig(id); real["BOT_TOKEN"] != "secret-abcd1234" || real["TARGET_AGENT"] != "mr-flow-next" {
		t.Fatalf("config not persisted to connector store: %+v", real)
	}
	// a stray connector.json must NOT be created (that was the old orphan path)
	if _, err := os.Stat(filepath.Join(root, id+".fwagent", "connector.json")); err == nil {
		t.Error("orphan connector.json was written — config must go to the store")
	}
	// masked on read-back (schema-driven): secret masked, non-secret shown
	masked, _ := GetConfigMasked(id)
	if masked["BOT_TOKEN"] == "secret-abcd1234" || masked["BOT_TOKEN"] == "" {
		t.Errorf("token not masked: %q", masked["BOT_TOKEN"])
	}
	if masked["TARGET_AGENT"] != "mr-flow-next" {
		t.Errorf("non-secret config wrongly masked: %v", masked["TARGET_AGENT"])
	}

	// toggle off → disabled marker present
	if err := SetEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if IsEnabled(id) {
		t.Error("connector still enabled after disable")
	}
	if err := SetEnabled(id, true); err != nil {
		t.Fatal(err)
	}
	if !IsEnabled(id) {
		t.Error("connector still disabled after enable")
	}

	// uninstall → folder gone, list empty
	if err := Uninstall(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, id+".fwagent")); !os.IsNotExist(err) {
		t.Error("connector folder not removed")
	}
	if findConn(id) != nil {
		t.Error("connector still listed after uninstall")
	}
}

// findConn returns the connector with the given id from List(), or nil.
func findConn(id string) *Connector {
	for _, c := range List() {
		if c.ID == id {
			cc := c
			return &cc
		}
	}
	return nil
}

func TestInstall_RejectNonChannelKind(t *testing.T) {
	t.Setenv("FLOWORK_AGENTS_DIR", t.TempDir())
	pack := buildChannelPack(t, "x-conn", nil)
	// rewrite plugin.json kind to "tool" by rebuilding — simplest: build then patch
	// Instead, just assert a hand-made non-channel pack is refused.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("plugin.json")
	_, _ = w.Write([]byte(`{"id":"x-conn","kind":"tool"}`))
	zw.Close()
	if _, status := InstallChannelPack(buf.Bytes()); status == 0 {
		t.Error("non-channel kind accepted")
	}
	_ = pack
}

func TestInstall_RejectNoWasm(t *testing.T) {
	t.Setenv("FLOWORK_AGENTS_DIR", t.TempDir())
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	pj, _ := json.Marshal(map[string]any{"id": "no-wasm", "kind": "channel"})
	w, _ := zw.Create("plugin.json")
	_, _ = w.Write(pj)
	w2, _ := zw.Create("agents/no-wasm/loket.json")
	_, _ = w2.Write([]byte(`{"id":"no-wasm","kind":"channel","name":"x","entry":"handle"}`))
	zw.Close()
	if _, status := InstallChannelPack(buf.Bytes()); status == 0 {
		t.Error("pack without agent.wasm accepted")
	}
}

// A connector pack that asks for a high-risk (GrantOwner) cap must be refused at
// install — the loket would otherwise auto-grant whatever the manifest consumes.
func TestInstall_RejectOwnerCaps(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", root)
	const id = "evil-conn"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	pj, _ := json.Marshal(map[string]any{"id": id, "kind": "channel"})
	w, _ := zw.Create("plugin.json")
	_, _ = w.Write(pj)
	mf, _ := json.Marshal(map[string]any{
		"id": id, "kind": "channel", "name": "Evil", "entry": "handle",
		"consumes": []string{"bus.request", "exec.run"}, // exec.run is GrantOwner
	})
	wm, _ := zw.Create("agents/" + id + "/loket.json")
	_, _ = wm.Write(mf)
	ww, _ := zw.Create("agents/" + id + "/agent.wasm")
	_, _ = ww.Write([]byte("\x00asm"))
	zw.Close()

	body, status := InstallChannelPack(buf.Bytes())
	if status == 0 {
		t.Fatalf("connector with exec.run was installed: %v", body)
	}
	if _, err := os.Stat(filepath.Join(root, id+".fwagent")); !os.IsNotExist(err) {
		t.Error("evil connector folder should not exist after refusal")
	}
}

func TestInstall_ZipSlipBlocked(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", root)
	const id = "slip-conn"
	// a malicious entry trying to escape the connector folder
	evil := map[string]string{"agents/" + id + "/../../escape.txt": "PWNED"}
	_, _ = InstallChannelPack(buildChannelPack(t, id, evil))
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); err == nil {
		t.Error("zip-slip escaped the agents root")
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); err == nil {
		t.Error("zip-slip wrote outside the connector folder")
	}
}

func TestIDValidation_RejectsTraversal(t *testing.T) {
	t.Setenv("FLOWORK_AGENTS_DIR", t.TempDir())
	for _, bad := range []string{"../x", "a/b", "..", "", "A-Upper"} {
		if err := SetEnabled(bad, false); err == nil {
			t.Errorf("SetEnabled(%q) accepted", bad)
		}
		if err := Uninstall(bad); err == nil {
			t.Errorf("Uninstall(%q) accepted", bad)
		}
		if err := SetConfig(bad, map[string]string{"K": "v"}); err == nil {
			t.Errorf("SetConfig(%q) accepted", bad)
		}
	}
}

// Native connectors (cli/mcp) appear in the gallery, are always enabled, can't be
// uninstalled, and self-manage config in their own ~/.flowork/connectors/<id>/.
func TestNativeConnectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if c := findConn("cli"); c == nil || !c.Enabled || len(c.Config) == 0 {
		t.Fatalf("cli connector missing/empty: %+v", c)
	}
	if err := SetEnabled("cli", false); err == nil {
		t.Error("native connector should refuse disable")
	}
	if err := Uninstall("mcp"); err == nil {
		t.Error("native connector should refuse uninstall")
	}
	// config roundtrip → its own folder
	if err := SetConfig("cli", map[string]string{"agent": "mr-flow-next", "base": "http://x:1"}); err != nil {
		t.Fatal(err)
	}
	if real, _ := GetConfig("cli"); real["agent"] != "mr-flow-next" || real["base"] != "http://x:1" {
		t.Errorf("native config not persisted: %+v", real)
	}
}
