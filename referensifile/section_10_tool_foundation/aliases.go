// aliases.go — name aliasing layer untuk reconcile GUI cap names dengan
// canonical registry tool names.
//
// Sprint 3.5g 2026-05-03 (post BUG-W58/W60/W61 doctrine): static analysis
// nemu 16 cap entry di role_capabilities table merpati yang TIDAK matching
// nama tool teregister di worker (`floworkos-go/internal/tools`).
// Mismatch ini bikin tool dispatch return ErrToolNotFound walau Ayah udah
// centang ON di GUI.
//
// Approach: alias map (cap-name -> canonical-name). Resolve dipanggil di
// Get() dan runViaWorker() — capability gate tetap matching nama original
// (cap stays whatever user toggled), tapi registry lookup + worker proxy
// pakai canonical name. Plug-and-play: nambah cap baru = nambah row di
// peta + tinggal redeploy. Ngga ngubah persisted cap state user.
//
// Doktrin: P-2 1-Pintu (semua dispatch lewat kernel). FQP-7 Plug-and-Play
// (config via map, ngga edit registry per-tool). FQP-5 No Wormhole (cap
// gate masih enforce, alias cuma resolve nama setelah gate lulus).

package tools

import "strings"

// canonicalAliases — old/synonym tool name -> canonical registered name.
//
// Source: static cross-reference role_capabilities (merpati) vs worker
// `Definition().Name` di floworkos-go/internal/tools (Sprint 3.5g audit).
//
// Aturan tambah entry:
//  1. CONFIRM canonical name ada di registry worker (via /v1/tool/list)
//     atau kernel local (kernel/tools/**.go init).
//  2. JANGAN alias 2-arah (multi_edit -> multiedit OK, tapi multiedit ->
//     multi_edit kebalikannya: kalau kedua cap aktif sekaligus akan loop).
//  3. Kalau target canonical udah dihapus dari registry, REMOVE alias-nya
//     (jangan biarkan dangling).
var canonicalAliases = map[string]string{
	// Underscore vs concat naming drift (GUI kasih underscore, registry concat)
	"ask_user_question":   "askuserquestion",
	"multi_edit":          "multiedit",
	"notebook_edit":       "notebookedit",
	"web_fetch":           "webfetch",
	"web_search":          "websearch",
	"todo_write":          "todo",

	// brain_remember — alias ke memorize_brain canonical.
	// Per opus-2 Issue 4 + opus-3 A-1 review 2026-05-08: capability uplift
	// commit e2f242c INSERT 4 row brain_remember ke cs/customer-service/
	// public/pelayan. Tool ngga eksist di registry (cuma memorize_brain
	// canonical). Tanpa alias = ErrToolNotFound. Alias resolve naming.
	"brain_remember": "memorize_brain",

	// Renamed tools (legacy cap masih ada)
	"ast_search":           "search_semantic_function",
	"browser_screenshot":   "screenshot",
	"claude_dispatch":      "warga_dispatch",
	"get_claude_training":  "get_external_training",
	"list_claude_training": "list_external_training",
	"mcp":                  "mcp_call",
	"sandbox":              "run_sandbox_command",

	// Cross-tier — kernel-side namespaced tool exposed via flat cap
	"wallet_read": "wallet.balance_read",

	// `git` dan `skill_write` sekarang teregister di worker (Sprint 3.5g
	// 2026-05-03) — `git` = generic dispatcher (status/log/diff/branch +
	// redirect write), `skill_write` = bootstrap SKILL.md. Tidak butuh
	// alias entry — direct registry lookup match.

	// Mr.Dev mandate 2026-05-21: training corpus c5 nge-bake PascalCase
	// tool names (Mr.Flow style mimicry industri-standar). Kernel registry
	// pake snake_case — bridge gap di sini supaya Mr.Flow halu PascalCase
	// auto-route ke real registered tool.
	//
	// Aturan: TARGET wajib ada di registry. Kalau target ngga eksis
	// (mis. SlashCommand belum di-register), JANGAN tambah alias — biar
	// salvage_textjson filter drop nama halu itu.
	"Bash":             "bash",
	"BashOutput":       "bash",
	"Read":             "read",
	"Write":            "write",
	"Edit":             "edit",
	"Glob":             "glob",
	"Grep":             "grep",
	"NotebookEdit":     "notebookedit",
	"WebSearch":        "websearch",
	"WebFetch":         "webfetch",
	"EnterPlanMode":    "enter_plan_mode",
	"ExitPlanMode":     "exit_plan_mode",
	"EnterWorktree":    "enter_worktree",
	"ExitWorktree":     "exit_worktree",
	"CronCreate":       "cron_create",
	"CronDelete":       "cron_delete",
	"CronList":         "cron_list",
	"TaskOutput":       "task_output",
	"TaskStop":         "task_stop",
	"TodoWrite":        "todo",
	"Monitor":          "monitor",
	"ToolSearch":       "tool_search",
	"RemoteTrigger":    "remote_trigger",
	"Skill":            "skill",
	"AskUserQuestion":  "askuserquestion",
	"AgentTask":        "agent_task_list", // semantic invent agent dispatch → list trio entry

	// Mr.Dev 2026-05-21 arch diagram subagent + slash:
	"AgentTool":      "delegate_task",  // Claude-Code naming → kernel canonical
	"agent_spawn":    "delegate_task",
	"spawn_subagent": "delegate_task",
	"SlashCommand":   "slash_command",  // Claude-Code naming → kernel canonical
	"slash":          "slash_command",
}

// ResolveAlias return canonical tool name kalau toolName ada di alias map,
// kalau ngga return original toolName. Idempotent — alias chain max depth 1
// (lihat aturan #2 di canonicalAliases doc).
//
// Caller pattern:
//
//	canonical := tools.ResolveAlias(toolName)
//	tool, err := tools.Get(canonical)
//
// Atau langsung di Get() (current implementation).
func ResolveAlias(toolName string) string {
	if canonical, ok := canonicalAliases[toolName]; ok {
		return canonical
	}
	// Strip kernel namespace prefix (hak_warga.inventory_read -> inventory_read)
	// supaya alias resolution juga handle namespace drop. Sinkron dengan
	// stripKernelNamespace di worker_proxy.go (BUG-W60 fix Sprint 3.5g).
	if idx := strings.LastIndex(toolName, "."); idx >= 0 && idx < len(toolName)-1 {
		stripped := toolName[idx+1:]
		if canonical, ok := canonicalAliases[stripped]; ok {
			return canonical
		}
	}
	return toolName
}
