// Package tools — mapping tool name → required capability.
//
// BUG-093 fix: F6 capability_check sebelumnya MVP placeholder default
// allowed=true (cuma cek bash/git). Refactor jadi proper mapping +
// zero-trust default deny untuk tool yang belum di-mapping.
//
// Pattern: tool name pattern (exact atau prefix) → capability string yang
// warga harus punya. Mirror tool taxonomy di kernel/tools/.

package tools

import "strings"

// capabilityMap maps tool name (exact match) → required capability.
// Order: spesifik dulu, fallback prefix di RequiredCapability.
var capabilityMap = map[string]string{
	// filesystem read
	"read_file":  "read",
	"list_files": "read",
	"glob":       "read",
	"grep":       "read",

	// filesystem write
	"write_file": "write",
	"edit_file":  "write",
	"multiedit":  "write",

	// filesystem delete
	"delete_file": "delete",

	// execution
	"bash":         "bash",
	"bash_execute": "bash",
	"bash_stream":  "bash",
	"python":       "python",

	// git
	"git.checkpoint": "git",
	"git.commit":     "git",
	"git.rollback":   "git",
	"git.verify":     "git",

	// browser (split per BUG-069 + BUG-078)
	"browser_navigate":   "browser_read",
	"browser_extract":    "browser_read",
	"browser_screenshot": "browser_read",
	"browser_click":      "browser_interact",
	"browser_eval":       "browser_eval", // arbitrary JS exec, restricted

	// workspace
	"workspace_read":   "read",
	"workspace_write":  "write",
	"workspace_list":   "read",
	"workspace_delete": "delete",

	// communication
	// forum_post, dream_post REMOVED 2026-05-25 (zombie — tools tidak registered di worker)
	"telegram_send": "notify",

	// brain
	"memorize_brain":   "brain",
	"brain_search":     "brain",
	"brain_recall":     "brain",
	// hunting_bug 2026-04-30 BUG-030: removed brain_kg_query + brain_kg_add_triple
	// — DROPPED rc189 per quality_control + README. Replaced by drawer system.
	// Capability map masih ngerujuk → tools.List() exposes deprecated tools ke
	// system prompt → Merpati klaim punya tool yang ngga ada implementasinya.
	"fact_remember":    "brain",
	"fact_recall":      "brain",
	"fact_forget":      "brain",

	// hak warga — flat names (worker-side stub) + namespaced aliases (kernel-side
	// hak_warga package). Sprint 3.5g BUG-W60 fix: dual-key biar buildToolSpecs
	// matching ngga gagal saat kernel register tool dengan name "hak_warga.X"
	// tapi GUI bridge return cap "X" (flat).
	// vote_cast, death_letter_read/write REMOVED 2026-05-25 (zombie, tools tidak registered)
	"bug_report":                   "hak_warga",
	"inventory_read":               "hak_warga",
	"daily_reflection":             "hak_warga",
	"roadmap_write":                "hak_warga",
	"tool_propose":                 "hak_warga",
	"change_log_post":              "hak_warga",
	"change_persona":               "hak_warga",
	"hak_warga.bug_report":         "hak_warga",
	"hak_warga.inventory_read":     "hak_warga",
	"hak_warga.daily_reflection":   "hak_warga",
	"hak_warga.roadmap_write":      "hak_warga",
	"hak_warga.tool_propose":       "hak_warga",
	"hak_warga.change_log_post":    "hak_warga",
	"hak_warga.change_persona":     "hak_warga",

	// cron
	"cron.add":     "cron",
	"cron.list":    "cron",
	"cron.remove":  "cron",
	"cron.trigger": "cron",

	// CRG (Code Review Graph) 2026-04-30 — read-only blast radius analysis
	"code_review_context": "read",
	// GitNexus G1+G3 2026-04-30 — Cypher-lite graph query + execution flow trace
	"code_graph_query": "read",
	"code_flow_trace":  "read",

	// Wallet 2026-05-01 — read-only OpenRouter saldo (per Ayah directive
	// "pastikan agar semua ai bisa baca sisa saldo open router"). Cuma READ
	// dari cached state file, no API call. Ayah toggle via GUI Hak dan Tools
	// — capability "wallet_read".
	"wallet.balance_read": "wallet_read",

	// Finance 2026-05-08 (Ayah mega-audit A6) — read-only laporan keuangan
	// (wallet latest + revenue/expense 7-hari + net) dari brain DB via GUI
	// bridge. Doktrin id 7362 E-6 mandate akses transparan untuk seluruh warga.
	// Default seeded untuk role akuntan/trader/admin (financial-aware roles).
	"finance.summary_read": "finance_read",

	// 2026-05-23 Linux migration — slash_command, memory, plan, task, agent,
	// mcp, tool_search. Sebelum ini ngga di-mapping → zero-trust deny saat
	// Mr.Flow dispatch via /v1/chat. Per Mr.Dev mandate: warga mr.flow harus
	// bisa dispatch semua tools untuk Telegram bot integration.
	"slash_command":     "slash",
	"tool_search":       "tool_search",
	"memory_get":        "memory",
	"memory_remember":   "memory",
	"plan_create":       "plan",
	"plan_get":          "plan",
	"task_create":       "task",
	"task_list":         "task",
	"task_get":          "task",
	"task_stop":         "task",
	"task_output":       "task",
	"agent_task_get":    "agent",
	"agent_task_list":   "agent",
	"agent_task_spawn":  "agent",
	"delegate_task":     "agent",
	"mcp_call":          "mcp",
	"mcp_list_servers":  "mcp",
	"todo":              "task",
	"todo_write":        "task",
}

// capabilityPrefixMap fallback kalau exact name ngga match — match prefix.
// Order: panjang ke pendek (lebih spesifik prioritas).
var capabilityPrefixMap = []struct {
	Prefix string
	Cap    string
}{
	{"browser_", "browser_read"}, // catch-all browser_* (kecuali eval/click sudah explicit)
	{"workspace_", "read"},       // workspace_* default ke read (write/delete sudah explicit)
	{"git.", "git"},
	{"brain_", "brain"},
	{"fact_", "brain"},
	{"forum_", "forum"},
	{"dream_", "forum"},
	{"cron.", "cron"},
	{"bash", "bash"},
	{"python", "python"},
	{"hak_warga.", "hak_warga"}, // semua hak_warga.* tools — konstitusional
}

// RequiredCapability return capability yang warga harus punya untuk tool name.
// Return ("", false) kalau tool unknown — caller wajib zero-trust DENY.
func RequiredCapability(toolName string) (string, bool) {
	if cap, ok := capabilityMap[toolName]; ok {
		return cap, true
	}
	for _, p := range capabilityPrefixMap {
		if strings.HasPrefix(toolName, p.Prefix) {
			return p.Cap, true
		}
	}
	return "", false
}
