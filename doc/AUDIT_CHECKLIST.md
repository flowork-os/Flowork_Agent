# Flowork Agent — AUDIT CHECKLIST

> Auto-generated 2026-05-30 per Mr.Dev mandate "audit setiap file, cari bug, perbaiki, lock".
> Status: 🔒 LOCKED + verified · ⏳ pending · 🛑 RESERVED future.

## Inventory

- **Go**: 111 file (111 🔒 = **100%**)
- **JS (web/)**: 19 file (3 🔒)

## 1. Completeness gap (port dari referensi)

| Component | Referensi | Kita | Missing |
|---|---|---|---|
| Tools (internal/tools/) | 112 | 24 | **88** |
| Scanner auditors | 109 | 6 | **103** |
| Slash commands | TBD recount | 12+3 | TBD |

## 2. Go files audit

### agents/mr-flow

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `agents/mr-flow/main.go` | 847 | audit pass 2026-05-30 |

### cmd

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `cmd/flowork-cli/main.go` | 180 | audit pass 2026-05-30 |

### internal/agentdb

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/agentdb/accessor.go` | 29 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/agentdb.go` | 814 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/audit.go` | 218 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/codemap.go` | 139 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/death_letter.go` | 244 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/decisions.go` | 182 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/edu_errors.go` | 165 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/finance.go` | 243 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/interactions.go` | 167 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/karma.go` | 190 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/mistakes.go` | 241 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/mistakes_promote.go` | 107 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/protector.go` | 235 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/retention.go` | 275 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/scanner.go` | 194 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/scheduler.go` | 225 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/slash_invocations.go` | 102 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/tool_audit.go` | 264 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/tool_invocations.go` | 154 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/tool_memory.go` | 114 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/tool_subscriptions.go` | 150 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/wallet.go` | 170 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/wallet_alert.go` | 201 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/workspace_meta.go` | 545 | audit pass 2026-05-30 |
| 🔒 | `internal/agentdb/zombie_modes_prompt.go` | 231 | audit pass 2026-05-30 |

### internal/agentmgr

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/agentmgr/agentmgr.go` | 1358 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/approval.go` | 117 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/audit.go` | 121 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/codemap.go` | 130 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/finance.go` | 239 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/legacy_compat.go` | 598 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/legacy_compat_v2.go` | 100 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/legacy_compat_v3.go` | 282 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/mesh.go` | 98 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/protector.go` | 182 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/router_skills.go` | 146 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/scanner.go` | 202 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/scheduler.go` | 92 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/sec29_35.go` | 254 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/sneakernet.go` | 141 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/tool_subscriptions.go` | 329 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/wallet.go` | 166 | audit pass 2026-05-30 |
| 🔒 | `internal/agentmgr/wallet_alert.go` | 124 | audit pass 2026-05-30 |

### internal/codemap

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/codemap/goparser.go` | 96 | audit pass 2026-05-30 |

### internal/httpx

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/httpx/json.go` | 43 | audit pass 2026-05-30 |

### internal/kernel

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/kernel/broker/broker.go` | 87 | audit pass 2026-05-30 |
| 🔒 | `internal/kernel/loader/manifest.go` | 410 | audit pass 2026-05-30 |
| 🔒 | `internal/kernel/loader/scanner.go` | 127 | audit pass 2026-05-30 |
| 🔒 | `internal/kernel/loader/watcher.go` | 153 | audit pass 2026-05-30 |
| 🔒 | `internal/kernel/runtime/host.go` | 736 | audit pass 2026-05-30 |
| 🔒 | `internal/kernel/runtime/instance.go` | 197 | audit pass 2026-05-30 |
| 🔒 | `internal/kernel/runtime/runtime.go` | 86 | audit pass 2026-05-30 |
| 🛑 | `internal/kernel/uimount/uimount.go` | 210 | RESERVED |

### internal/kernelhost

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/kernelhost/kernelhost.go` | 1245 | audit pass 2026-05-30 |

### internal/protector

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/protector/baseline.go` | 140 | audit pass 2026-05-30 |

### internal/routerclient

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/routerclient/brain_search.go` | 85 | audit pass 2026-05-30 |
| 🔒 | `internal/routerclient/mesh.go` | 95 | audit pass 2026-05-30 |
| 🔒 | `internal/routerclient/normalize.go` | 46 | audit pass 2026-05-30 |
| 🔒 | `internal/routerclient/retry.go` | 220 | audit pass 2026-05-30 |
| 🔒 | `internal/routerclient/routerclient.go` | 183 | audit pass 2026-05-30 |
| 🔒 | `internal/routerclient/skills.go` | 140 | audit pass 2026-05-30 |

### internal/scanner

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/scanner/auditors.go` | 262 | audit pass 2026-05-30 |
| 🔒 | `internal/scanner/runner.go` | 144 | audit pass 2026-05-30 |

### internal/scheduler

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/scheduler/cron.go` | 170 | audit pass 2026-05-30 |
| 🔒 | `internal/scheduler/cron_test.go` | 86 | audit pass 2026-05-30 |
| 🔒 | `internal/scheduler/engine.go` | 271 | audit pass 2026-05-30 |

### internal/slashcmd

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/slashcmd/builtins/builtins.go` | 92 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/builtins/tier1.go` | 232 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/builtins/tool_search.go` | 75 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/context.go` | 65 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/custom/loader.go` | 216 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/custom/watcher.go` | 196 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/dispatcher.go` | 64 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/hooks.go` | 230 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/registry.go` | 116 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/registry_dynamic.go` | 56 | audit pass 2026-05-30 |
| 🔒 | `internal/slashcmd/types.go` | 49 | audit pass 2026-05-30 |

### internal/sneakernet

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/sneakernet/export.go` | 223 | audit pass 2026-05-30 |
| 🔒 | `internal/sneakernet/import.go` | 158 | audit pass 2026-05-30 |
| 🔒 | `internal/sneakernet/manifest.go` | 46 | audit pass 2026-05-30 |
| 🔒 | `internal/sneakernet/verify.go` | 74 | audit pass 2026-05-30 |

### internal/tools

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/tools/builtins/brain.go` | 106 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/builtins.go` | 232 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/codemap_tools.go` | 114 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/file.go` | 252 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/file_advanced.go` | 330 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/git.go` | 134 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/orchestration.go` | 320 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/shell.go` | 253 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/shell_rlimit_linux.go` | 69 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/shell_rlimit_other.go` | 26 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/skill.go` | 146 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/telegram.go` | 170 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/builtins/web.go` | 179 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/context.go` | 104 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/interceptors.go` | 296 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/registry.go` | 124 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/sandbox.go` | 144 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/sandbox_v3.go` | 217 | audit pass 2026-05-30 |
| 🔒 | `internal/tools/types.go` | 105 | audit pass 2026-05-30 |

### internal/wallet

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/wallet/coingecko.go` | 146 | audit pass 2026-05-30 |
| 🔒 | `internal/wallet/etherscan.go` | 346 | audit pass 2026-05-30 |
| 🔒 | `internal/wallet/portfolio.go` | 224 | audit pass 2026-05-30 |
| 🔒 | `internal/wallet/tokens.go` | 97 | audit pass 2026-05-30 |

### internal/walletalert

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/walletalert/evaluator.go` | 189 | audit pass 2026-05-30 |

### internal/watchdog

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/watchdog/evaluator.go` | 193 | audit pass 2026-05-30 |

### internal/zombie

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `internal/zombie/detector.go` | 189 | audit pass 2026-05-30 |

### root

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `main.go` | 418 | audit pass 2026-05-30 |

### sdk

| Status | File | LOC | Notes |
|---|---|---|---|
| 🔒 | `sdk/go/echo/main.go` | 70 | audit pass 2026-05-30 |

## 3. JS files audit

| Status | File | LOC | Notes |
|---|---|---|---|
| ⏳ | `web/js/app.js` | 257 | pending |
| ⏳ | `web/js/i18n.js` | 92 | pending |
| ⏳ | `web/js/splitlist.js` | 120 | pending |
| ⏳ | `web/js/utils.js` | 529 | pending |
| ⏳ | `web/tabs/agents.js` | 922 | pending |
| 🔒 | `web/tabs/agents_router_skills.js` | 147 | audit pass |
| 🔒 | `web/tabs/agents_slash_modal.js` | 117 | audit pass |
| 🔒 | `web/tabs/agents_tool_catalog.js` | 90 | audit pass |
| ⏳ | `web/tabs/codemap.js` | 966 | pending |
| ⏳ | `web/tabs/commits.js` | 36 | pending |
| ⏳ | `web/tabs/diagnostics.js` | 604 | pending |
| ⏳ | `web/tabs/doktrin_edukasi.js` | 310 | pending |
| ⏳ | `web/tabs/finance.js` | 417 | pending |
| ⏳ | `web/tabs/prompt.js` | 260 | pending |
| ⏳ | `web/tabs/protector.js` | 425 | pending |
| ⏳ | `web/tabs/scanner.js` | 573 | pending |
| ⏳ | `web/tabs/wallet.js` | 414 | pending |
| ⏳ | `web/tabs/warga_caps.js` | 272 | pending |
| ⏳ | `web/vendor/d3.min.js` | 2 | pending |