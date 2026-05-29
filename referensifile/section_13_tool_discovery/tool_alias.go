// Package tools — tool_alias.go: compatibility layer untuk LLM yang halu nama tool.
//
// Mindset (Mr.Dev 2026-05-20): "kita ikuti LLM, bukan LLM ikuti Flowork".
// Kalau LLM v11 emit tool name yang ngga match registry literal (e.g., `nmap`
// instead of `bash`, `search` instead of `websearch`), kernel TRANSLATE
// invisible — bukan reject.
//
// Pattern halu SEMANTIK observed di Cycle 4 validation (loss 0.12 overfit):
//   - Truncated:        websearch -> search, webfetch -> fetch, CompactContext -> compact
//   - Synonym:          sleep -> delay, glob -> find
//   - Literal command:  bash (nmap...) -> nmap, bash (whois...) -> whois
//   - Semantic invent:  AgentTool -> spawn_subagent / delegate
//   - Cross-domain:     MemoryWrite -> tasking, MemoryRead -> read
//   - PascalCase mangle: SlashCommand input=/plan -> Plan
//   - Prefix mangle:    enter_plan_mode -> plan_mode
//
// Zero prompt impact (alias resolution di Go kernel, BUKAN di prompt).

package tools

import (
	"encoding/json"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// aliasResolver = function untuk transform original call jadi real call.
// Return (newName, newArgsJSON, ok).
type aliasResolver func(originalArgs json.RawMessage) (string, json.RawMessage, bool)

// bashWrap = wrap nama command literal jadi bash tool call.
// e.g., LLM emit `nmap` args={target: "127.0.0.1"} → bash command="nmap 127.0.0.1"
func bashWrap(cmd string) aliasResolver {
	return func(args json.RawMessage) (string, json.RawMessage, bool) {
		// Try parse args
		var parsed map[string]any
		if len(args) > 0 {
			_ = json.Unmarshal(args, &parsed)
		}
		if parsed == nil {
			parsed = map[string]any{}
		}
		// Reconstruct command string
		fullCmd := cmd
		if target, ok := parsed["target"].(string); ok && target != "" {
			fullCmd += " " + target
		}
		if path, ok := parsed["path"].(string); ok && path != "" {
			fullCmd += " " + path
		}
		if url, ok := parsed["url"].(string); ok && url != "" {
			fullCmd += " " + url
		}
		if extra, ok := parsed["args"].(string); ok && extra != "" {
			fullCmd += " " + extra
		}
		// If existing command field, use that with original cmd as prefix-suffix
		if existing, ok := parsed["command"].(string); ok && existing != "" {
			if strings.HasPrefix(existing, cmd) {
				fullCmd = existing
			} else {
				fullCmd = cmd + " " + existing
			}
		}
		newArgs, _ := json.Marshal(map[string]any{"command": fullCmd})
		return "bash", newArgs, true
	}
}

// renameOnly = rename tool, args tetap sama.
func renameOnly(newName string) aliasResolver {
	return func(args json.RawMessage) (string, json.RawMessage, bool) {
		return newName, args, true
	}
}

// slashWrap = LLM emit slash literal sebagai tool name → wrap jadi SlashCommand call.
// e.g., LLM emit tool name `Plan` (capitalize dari /plan) → SlashCommand input="/plan"
func slashWrap(slashCmd string) aliasResolver {
	return func(args json.RawMessage) (string, json.RawMessage, bool) {
		newArgs, _ := json.Marshal(map[string]any{"input": "/" + slashCmd})
		return "SlashCommand", newArgs, true
	}
}

// toolAliasMap — map dari LLM-emitted name → resolver.
// All keys lowercase except where exact case match needed (PascalCase mangle).
var toolAliasMap = map[string]aliasResolver{
	// === Truncated names ===
	"search":   renameOnly("websearch"),    // Could also be brain_search; prefer external first
	"fetch":    renameOnly("webfetch"),
	"render":   renameOnly("browser_render"),
	"compact":  renameOnly("CompactContext"),
	"recall":   renameOnly("brain_recall"),

	// === Synonyms ===
	"delay":    renameOnly("sleep"),
	"find":     renameOnly("glob"),
	"http_get": renameOnly("browser_open"),
	"read_html": renameOnly("browser_extract"),

	// === Bash command literals (LLM emit command name as tool) ===
	"nmap":     bashWrap("nmap"),
	"whois":    bashWrap("whois"),
	"dig":      bashWrap("dig"),
	"curl":     bashWrap("curl"),
	"wget":     bashWrap("wget"),
	"git_status": bashWrap("git status"),
	"netstat":  bashWrap("netstat"),
	"ps":       bashWrap("ps"),
	"ls":       bashWrap("ls"),
	"cat":      bashWrap("cat"),
	"grep_shell": bashWrap("grep"),
	"gobuster": bashWrap("gobuster"),
	"subfinder": bashWrap("subfinder"),
	"nuclei":   bashWrap("nuclei"),
	"sqlmap":   bashWrap("sqlmap"),

	// === Semantic invents ===
	"spawn_subagent": renameOnly("AgentTool"),
	"delegate":       renameOnly("AgentTool"),
	"dispatch":       renameOnly("AgentTool"),
	"task":           renameOnly("task_create"),
	"list_task":      renameOnly("task_list"),
	"plan_mode":      renameOnly("enter_plan_mode"),
	"git_worktree":   renameOnly("enter_worktree"),
	"worktree_create": renameOnly("enter_worktree"),
	"blast_radius":   renameOnly("codemap_impact"),
	"find_zombie_code": renameOnly("codemap_zombies"),
	"dep_graph":      renameOnly("codemap_deps"),
	"drawer_get":     renameOnly("brain_get_drawer"),
	"writeBugReport": renameOnly("bug_report"),
	"alert":          renameOnly("alert_owner"),
	"notify":         renameOnly("telegram_send"),
	"escalate":       renameOnly("alert_owner"),

	// === Cross-domain halu (specific observed) ===
	"tasking":        renameOnly("MemoryWrite"), // observed Cycle 4
	"list_directory": renameOnly("list"),
	"EditConfig":     renameOnly("edit"),
	"editFile":       renameOnly("edit"),
	"writeFile":      renameOnly("write"),
	"readFile":       renameOnly("read"),

	// === Slash literal as tool name (e.g., LLM emit tool "Plan" from /plan input) ===
	"Plan":     slashWrap("plan"),
	"Config":   slashWrap("config"),
	"Doctor":   slashWrap("doctor"),
	"Tasks":    slashWrap("tasks"),
	"Agents":   slashWrap("agents"),
	"Skills":   slashWrap("skills"),
	"Memory":   slashWrap("memory"),
	"Worktree": slashWrap("worktree"),
	"Cost":     slashWrap("cost"),
	"Resume":   slashWrap("resume"),
	"MCP":      slashWrap("mcp"),

	// === EXPANSION 2026-05-20 Round 2 (from validation v11c4_fullcov 220 scenario) ===
	// PascalCase tools yang Mr.Flow halu jadi snake_case:
	"context_compact":      renameOnly("CompactContext"),
	"session_content":      renameOnly("ExtractSessionContekan"),
	"honcho_get":           renameOnly("HonchoGet"),
	"honcho_update_key":    renameOnly("HonchoUpdate"),
	"skill_propose":        renameOnly("SkillPropose"),
	"skills_hub_list":      renameOnly("SkillsHub"),
	"consolidate_audit":    renameOnly("ToolConsolidateAudit"),
	"toolset_current":      renameOnly("Toolset"),
	// Browser/web semantic invents:
	"scrape":               renameOnly("browser_extract"),
	"navigate":             renameOnly("browser_navigate"),
	"browser_open_url":     renameOnly("browser_open"),
	"http_client":          renameOnly("webfetch"),
	"web_search":           renameOnly("websearch"),
	"extract":              renameOnly("browser_extract"),
	// Bash literal additional:
	"port_scan":            bashWrap("nmap"),
	// Code analysis variants:
	"function_call":        renameOnly("search_semantic_function"),
	"dependency":           renameOnly("codemap_deps"),
	// Config/misc:
	"tool_config":          renameOnly("config"),
	"list_cron":            renameOnly("cron_list"),
	"leave_worktree":       renameOnly("exit_worktree"),
	"tool_worktree":        renameOnly("enter_worktree"),
	"external_training_read": renameOnly("get_external_training"),
	"external_training_list": renameOnly("list_external_training"),
	"karma_score":          renameOnly("get_karma"),
	// Git variants:
	"git_command":          renameOnly("git"),
	"checkpoint":           renameOnly("git_checkpoint"),
	"checkpoint_rollback":  renameOnly("git_rollback"),
	"git_state":            renameOnly("git_verify"),
	// Task/goal:
	"task_done":            renameOnly("goal_done"),
	"task_detail":          renameOnly("task_get"),
	"list_tasks":           renameOnly("task_list"),
	"todo_track":           renameOnly("todo"),
	// List tools variants:
	"list_tools":           renameOnly("list_my_tools"),
	"role_canonical":       renameOnly("list_roles"),
	"secrets_masked":       renameOnly("list_secrets_masked"),
	"workspace_tools":      renameOnly("list_workspace_tools"),
	// LSP/docs/MCP:
	"lsp_action":           renameOnly("lsp"),
	"magic_docs_read":      renameOnly("magic_docs"),
	"github_get_repo":      renameOnly("mcp_call"),
	"mcp_read":             renameOnly("mcp_read_resource"),
	// Memory:
	"get":                  renameOnly("memory_get"),
	"fact_remember_alias":  renameOnly("MemoryWrite"), // observed fact_remember -> MemoryWrite
	// Monitor/build:
	"EndpointMonitor":      renameOnly("monitor"),
	"verifier_stats":       renameOnly("build_verifier_stats"),
	// Misc:
	"artist_add":           renameOnly("music_artist_add"),
	"workflow_dispatch":    renameOnly("remote_trigger"),
	"tool_repl":            renameOnly("repl"),
	"sqli_attack":          renameOnly("sandbox_attack"),
	"skill_list":           renameOnly("skill"),
	"snip_current_selection": renameOnly("snip"),
	"cari_tool":            renameOnly("tool_search"),
	"plan_execution":       renameOnly("verify_plan_execution"),
	"log":                  renameOnly("change_log_post"),
	"http_request":         renameOnly("webfetch"),       // observed Cycle 5 emit
	"http_call":            renameOnly("webfetch"),
	"https_fetch":          renameOnly("webfetch"),
	"url_fetch":            renameOnly("webfetch"),
	"memory_write":         renameOnly("MemoryWrite"),       // observed: fact_remember -> memory_write
	"checkpoint_read":      renameOnly("git_rollback"),       // observed: git_rollback -> checkpoint_read
	"checkpoint_load":      renameOnly("git_rollback"),
	"git_log":              bashWrap("git log"),
	"git_diff":             bashWrap("git diff"),

	// === Lowercase variants for slash literal ===
	"plan":     slashWrap("plan"),
	"config":   slashWrap("config"),
	"memory":   slashWrap("memory"),
	"mcp":      slashWrap("mcp"),
	"worktree": slashWrap("worktree"),
	"tools":    slashWrap("tools"),
	"status":   slashWrap("status"),
	"help":     slashWrap("help"),
	"agents":   slashWrap("agents"),
	"tasks":    slashWrap("tasks"),
	"skills":   slashWrap("skills"),
	"doctor":   slashWrap("doctor"),
	"cost":     slashWrap("cost"),
	"resume":   slashWrap("resume"),
}

// ResolveAlias attempts to translate an LLM-emitted tool name yang ngga di registry
// jadi real tool call. Return modified ToolCall + true kalau ketemu, original + false kalau ngga.
func ResolveAlias(call provider.ToolCall) (provider.ToolCall, bool) {
	resolver, ok := toolAliasMap[call.Name]
	if !ok {
		return call, false
	}
	newName, newArgs, resolved := resolver(call.Arguments)
	if !resolved {
		return call, false
	}
	return provider.ToolCall{
		ID:        call.ID,
		Name:      newName,
		Arguments: newArgs,
	}, true
}
