// 🔒 FROZEN tools-core — Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// JANGAN edit tanpa unfreeze SADAR owner (di-hash KERNEL_FREEZE.md + chattr +i). Baca lock/tools.md DULU.
// Codegen + kebijakan import ada di toolsidecar.go/_ext.go — perluasan lewat sana, bukan file ini.
//
// tool_create.go — SELF-EVOLVING: agent BIKIN tool sidecar SENDIRI (owner 2026-06-23).
//
// Tool lahir PRIVAT (cuma pembuat liat+pake) → lolos Dewan self-evolution → jadi shared semua agent.
// Logic di internal/toolsidecar (CreateTool: scaffold+build-verify+register). Blueprint: roadmap §15.
// File baru non-frozen (cara nambah tool tanpa bongkar kernel frozen).
package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
	"flowork-gui/internal/toolsidecar"
)

func init() { tools.Register(&toolCreateTool{}) }

type toolCreateTool struct{}

func (toolCreateTool) Name() string { return "tool_create" }

// Capability "" = SEMUA agent boleh bikin tool (visi owner). Aman karena: tool lahir PRIVAT +
// guard import-eskalasi + Dewan-review sebelum jadi shared (roadmap §15.3).
func (toolCreateTool) Capability() string { return "" }

func (toolCreateTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Bikin tool sidecar SENDIRI. Lahir PRIVAT (cuma kamu pakai) sampai lolos Dewan→shared. `code` = badan " +
			"`func run(args map[string]any) (any, string)`: ambil param dari args, balikin (output, errString). Gagal compile→" +
			"build_log balik, perbaiki & ulang. DILARANG os/exec/syscall/unsafe (pure-compute; json/io/os auto-import).",
		Params: []tools.Param{
			{Name: "name", Type: tools.ParamString, Description: "nama UNIK ^[a-z][a-z0-9_]{1,39}$ (ga nimpa tool lain)", Required: true},
			{Name: "description", Type: tools.ParamString, Description: "deskripsi + kapan dipakai", Required: true},
			{Name: "code", Type: tools.ParamString, Description: "badan func run(args map[string]any)(any,string)", Required: true},
			{Name: "params", Type: tools.ParamArray, Description: "[{name,type,description,required}]"},
			{Name: "imports", Type: tools.ParamArray, Description: "import Go ekstra, mis [\"strings\",\"regexp\"]"},
			{Name: "capability", Type: tools.ParamString, Description: "kosongin (default). Privileged=review lebih ketat"},
			{Name: "returns", Type: tools.ParamString, Description: "bentuk output"},
		},
		Returns: "{ok, name, scope, build_log?}",
	}
}

func (toolCreateTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	agentID := strings.TrimSpace(tools.FromAgent(ctx))
	if agentID == "" {
		return tools.Result{}, fmt.Errorf("agent id ga kebaca (tool wajib ber-owner)")
	}
	cs := toolsidecar.CreateSpec{
		Name:        tcStr(args["name"]),
		Description: tcStr(args["description"]),
		Capability:  tcStr(args["capability"]),
		Returns:     tcStr(args["returns"]),
		Code:        tcStr(args["code"]),
		Imports:     tcStrSlice(args["imports"]),
		Params:      tcParams(args["params"]),
	}
	buildLog, err := toolsidecar.CreateTool(toolsidecar.ToolsDir(), agentID, cs)
	if err != nil {
		// Gagal compile/validasi → balikin {ok:false, error, build_log} biar agent BENERIN + retry
		// (BUKAN error-keras — ini loop belajar, bukan kegagalan tool).
		out := map[string]any{"ok": false, "name": cs.Name, "error": err.Error()}
		if strings.TrimSpace(buildLog) != "" {
			out["build_log"] = buildLog
		}
		return tools.Result{Output: out}, nil
	}
	return tools.Result{Output: map[string]any{
		"ok": true, "name": cs.Name, "scope": "private", "owner": agentID,
		"note": "Tool lahir PRIVAT (cuma kamu yg pake). Bakal di-review Dewan buat jadi shared semua agent.",
	}}, nil
}

func tcStr(v any) string { s, _ := v.(string); return strings.TrimSpace(s) }
func tcStrSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, e := range arr {
		if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}
func tcParams(v any) []toolsidecar.CreateParam {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []toolsidecar.CreateParam{}
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		req, _ := m["required"].(bool)
		out = append(out, toolsidecar.CreateParam{
			Name: tcStr(m["name"]), Type: tcStr(m["type"]), Description: tcStr(m["description"]), Required: req,
		})
	}
	return out
}
