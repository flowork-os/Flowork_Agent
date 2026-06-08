// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-02
// Reason: Router delegation tool. E2E verified (Mr.Flow→agent_command→operator
//   -komputer→system_power dry-run→reply relayed back). cap rpc:agent-invoke
//   (router only), self-invoke rejected, host hook InvokeAgentFunc. Extend
//   (target whitelist / depth cap) → tambah file baru, JANGAN modify ini.
//
// agent_command.go — delegation tool: let a router agent (Mr.Flow) hand a
// natural-language command to a specialist agent and relay its reply.
//
// WHY: the taskflow Category Task path is analysis-shaped (fan-out research →
// synthesize a BUY/HOLD/AVOID decision). It does NOT fit ACTION dispatch like
// "operate my computer". This tool is the action-dispatch counterpart: Mr.Flow
// stays the single front-door (Telegram/GUI), recognizes an operation request,
// and delegates it to a specialist agent that owns the right persona/tools. The
// specialist runs its OWN engine — its persona, its tools, its safety prompts —
// and returns a reply Mr.Flow relays back. NOTE: computer power/control is no
// longer delegated — it is a FIRST-CLASS Mr.Flow tool now (system_power), so the
// operator-komputer agent was retired. This tool remains for genuine specialist
// delegation, not power control.
//
// SECURITY: capability `rpc:agent-invoke` — only granted to the router agent.
// A normal agent can't invoke others. Self-invoke is rejected (no trivial
// loops); deeper recursion is blocked because the delegated agent lacks the
// capability. The host wires InvokeAgentFunc at boot (main.go) — nil until then.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

// InvokeAgentFunc — host hook (set in main.go = host.InvokeAgentMessage).
// Signature mirrors taskflow.Invoker so the same host method backs both.
var InvokeAgentFunc func(ctx context.Context, agentID, text, caller string) (string, error)

type agentCommandTool struct{}

func (agentCommandTool) Name() string       { return "agent_command" }
func (agentCommandTool) Capability() string { return "rpc:agent-invoke" }
func (agentCommandTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Delegate a natural-language command to a specialist agent and get its reply back. Use this to ROUTE a request to an agent that owns the right tools/persona instead of doing it yourself. NOTE: computer power/control (shutdown, restart, sleep, lock, logout) and opening apps are FIRST-CLASS tools now — use system_power / app_open directly, do NOT delegate those. This tool is for genuine specialist delegation. Pass the request through as text; relay the reply to the user verbatim.",
		Params: []tools.Param{
			{Name: "agent_id", Type: tools.ParamString, Description: "target specialist agent id", Required: true},
			{Name: "text", Type: tools.ParamString, Description: "the command / request in natural language", Required: true},
		},
		Returns: "{agent_id, reply}",
	}
}

func (agentCommandTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	target, _ := args["agent_id"].(string)
	target = strings.TrimSpace(target)
	text, _ := args["text"].(string)
	text = strings.TrimSpace(text)
	if target == "" || text == "" {
		return tools.Result{}, fmt.Errorf("agent_command: agent_id and text are required")
	}
	if InvokeAgentFunc == nil {
		return tools.Result{}, fmt.Errorf("agent_command: host invoke hook not wired")
	}
	// Reject self-invoke (no trivial loops).
	if from := tools.FromAgent(ctx); from != "" && strings.EqualFold(from, target) {
		return tools.Result{}, fmt.Errorf("agent_command: cannot delegate to self (%s)", target)
	}

	caller := "delegate:" + tools.FromCaller(ctx)
	reply, err := InvokeAgentFunc(ctx, target, text, caller)
	if err != nil {
		return tools.Result{}, fmt.Errorf("agent_command: invoke %q failed: %w", target, err)
	}
	return tools.Result{Output: map[string]any{
		"agent_id": target,
		"reply":    reply,
	}}, nil
}
