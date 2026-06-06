// tooladapter.go — TOOL-PACK plug-and-play (roadmap multi-KIND `tool`).
//
// Tool plugin = wasm "tool-agent" (kind:agent di AgentsDir, di-load kernel).
// WasmTool adapter implement tools.Tool; Run-nya invoke wasm lewat
// host.InvokeAgentMessage (REUSE — NOL ubah kernel/locked). Registrasi runtime
// via tools.RegisterDynamic (tools/dynamic.go). Marker `tool.json` di dir agent
// → boot scan re-register (persist tanpa DB).
//
// Litmus plug-and-play: drop .fwpack → tool langsung kepake SEMUA agent (via
// tool_search / tools/run); uninstall → ilang. Inti ga disentuh.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/tools"
)

// toolNameRe — nama tool: verb_noun lowercase (mis. text_stats, base64_encode).
var toolNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

// toolSpec — definisi 1 tool plugin (di plugin.json tool-pack + marker tool.json).
type toolSpec struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Capability  string        `json:"capability"`
	Params      []tools.Param `json:"params"`
	AgentID     string        `json:"agent_id"` // wasm tool-agent yang nge-eksekusi
}

// WasmTool — adapter tools.Tool yang nge-eksekusi tool lewat wasm tool-agent.
type WasmTool struct {
	spec toolSpec
	host *kernelhost.Host
}

func (t *WasmTool) Name() string       { return t.spec.Name }
func (t *WasmTool) Capability() string { return t.spec.Capability }
func (t *WasmTool) Schema() tools.Schema {
	return tools.Schema{Description: t.spec.Description, Params: t.spec.Params}
}

// Run — marshal args → invoke wasm tool-agent (handle_message) → parse Result.
func (t *WasmTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	if t.host == nil {
		return tools.Result{}, fmt.Errorf("tool %q: host ga ke-wire", t.spec.Name)
	}
	argsJSON, _ := json.Marshal(args)
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	raw, err := t.host.InvokeAgentMessage(cctx, t.spec.AgentID, string(argsJSON), "tool:"+t.spec.Name)
	if err != nil {
		return tools.Result{}, fmt.Errorf("invoke tool-agent %q: %w", t.spec.AgentID, err)
	}
	var res tools.Result
	if json.Unmarshal([]byte(raw), &res) != nil {
		// wasm ga balik shape Result → bungkus mentah biar ga ilang.
		return tools.Result{Output: raw}, nil
	}
	return res, nil
}

// toolMarkerPath — lokasi marker tool.json di dir agent tool.
func toolMarkerPath(agentID string) string {
	return filepath.Join(loader.AgentsDir(), agentID+".fwagent", "tool.json")
}

// registerWasmTool — register WasmTool ke registry + tulis marker (kalau persist).
func registerWasmTool(host *kernelhost.Host, spec toolSpec, persist bool) error {
	if spec.Name == "" || spec.AgentID == "" {
		return fmt.Errorf("toolSpec invalid (name/agent_id kosong)")
	}
	if err := tools.RegisterDynamic(&WasmTool{spec: spec, host: host}); err != nil {
		return err
	}
	if persist {
		raw, _ := json.MarshalIndent(spec, "", "  ")
		_ = os.WriteFile(toolMarkerPath(spec.AgentID), raw, 0o644)
	}
	return nil
}

// reregisterToolPacksOnBoot — scan AgentsDir buat marker tool.json → re-register
// SEMUA tool plugin pas boot (persist tanpa DB). Dipanggil dari main setelah host
// + kernel ready. Balik jumlah tool yang ke-register.
func reregisterToolPacksOnBoot(host *kernelhost.Host) int {
	root := loader.AgentsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(root, e.Name(), "tool.json"))
		if rerr != nil {
			continue
		}
		var spec toolSpec
		if json.Unmarshal(raw, &spec) != nil {
			continue
		}
		if err := registerWasmTool(host, spec, false); err == nil {
			n++
		} else {
			fmt.Fprintf(os.Stderr, "[tool-pack] boot re-register %q: %v\n", spec.Name, err)
		}
	}
	return n
}
