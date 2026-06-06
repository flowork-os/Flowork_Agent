# 🔗 ROADMAP — MCP Connector (Jenis 2: tool-source buat agent)

> **Status:** AKTIF (mulai 2026-06-06). Ditandai ✅ per phase yang SELESAI+TEST+PUSH.
>
> **Buat AI eksternal yang audit:** baca "KENAPA" + "CARA KERJA" dulu. Semua referensi fungsi di sini
> NYATA (udah dibaca dari kode), bukan halu. Owner nentuin GOL; arsitektur teknis diputusin AI.

---

## 🎯 GOL (dari owner — FIXED)

Connector ada **2 JENIS**:
1. **Channel** (telegram/cli/whatsapp/discord) — I/O manusia ↔ agent. *(udah dibangun, ROADMAP_CONNECTIONS.md)*
2. **MCP (tool-source)** — server MCP luar (github/filesystem/dll) yang **nyediain TOOL buat agent**.

Aturan akses (arahan owner): **default SEMUA agent bisa akses tool MCP**, tapi **bisa di-uncheck per-agent**
kalau ga butuh. *"Setiap user beda gaya main, harus fleksibel."* → opt-OUT, bukan opt-in.

---

## 🧠 KENAPA arsitektur ini (rationale)

### 1. MCP connector = "tool-source", nyolok ke tool-registry yang UDAH ADA — BUKAN sistem baru
- **Kenapa:** Flowork udah punya sistem tool lengkap (`internal/tools`: `RegisterDynamic`, `tool.specs`,
  `tool.run`, `tool_search`) yang tiap agent ngerti. MCP tool tinggal **didaftarin ke registry itu**.
  Bikin jalur tool baru = duplikasi + lawan "papan kosong".
- **Bukti pola:** `tools.RegisterDynamic(t Tool)` (internal/tools/dynamic.go:27) udah dipake tool-pack
  plug-and-play. MCP tool = dynamic tool juga.

### 2. Default-on lewat `tool_search`, BUKAN di-dump ke prompt → fleksibel TANPA over-prompt
- **Kenapa:** kalo 30 tool github di-cekokin ke system-prompt tiap agent → semut kebanjiran (akar refactor
  11×, anti-over-prompt). Tapi owner mau semua agent BISA akses. Solusi: MCP tool masuk registry → ke-temu
  via **`tool_search` on-demand** (`ToolSpecsHandler` cuma expose core ~13 + subscribed; sisanya via search,
  internal/agentmgr/tool_specs.go:13). Jadi default semua agent bisa pake, prompt tetep mungil.

### 3. Uncheck per-agent = pola subscription yang UDAH ADA (state.db agent sendiri)
- **Kenapa:** Flowork udah punya per-agent tool subscription (`SubscribeTool`/`UnsubscribeTool`/
  `SubscribedSet`, tool_subscriptions.go) di state.db tiap agent (terisolasi). "Uncheck MCP github di agent X"
  = exclusion per-agent di state.db X. Konsisten + isolasi (rusak 1 agent = state.db dia doang).

### 4. Self-managed config + owner-approved (keamanan)
- **Kenapa:** server MCP = proses host dengan akses sesuai token (github-mcp hit API github, fs-mcp baca file)
  = **high-risk**. Jadi **owner yang install + kasih token** (GrantOwner model). AI ga bisa nambah MCP sendiri.
  Config (command/args/env+token) disimpen di folder connector MCP sendiri (pola self-manage Channels).

### 5. Eksekusi tetep lewat sandbox
- Tool MCP pas di-run lewat `SandboxRunV3` (jalur `tool.run` yang udah ada) → capability gate + consent
  tetep jalan. MCP bukan bypass, tambahan tool-source di atas gate yang ada.

---

## ⚙️ STRUKTUR (komponen baru, terisolasi)

| Komponen | Lokasi | Guna |
|---|---|---|
| **MCP client** | `internal/mcpclient/` (BARU) | spawn server MCP (stdio JSON-RPC) · initialize · tools/list · tools/call · reap |
| **MCP registry** | `internal/connections/` (extend) | `kind:mcp` connector: config {command,args,env} di folder sendiri · install/list/enable/uninstall |
| **Tool bridge** | `internal/mcpclient/` → `tools.RegisterDynamic` | tiap tool MCP → Flowork Tool (`Run()` manggil `tools/call`), nama `mcp_<conn>_<tool>` |
| **Per-agent uncheck** | `tool_subscriptions.go` pola + state.db agent | exclusion connector MCP per-agent (default semua aktif) |
| **GUI** | `web/tabs/connections.js` | 2 kategori: Channels + MCP. Install MCP = tempel JSON `mcpServers` + token |

---

## 🔄 CARA KERJA (alur end-to-end, fungsi nyata)

**Install (owner):** Connections → MCP → tempel `{ "github": {"command":"npx","args":[...],"env":{"GITHUB_TOKEN":"..."}} }`
→ disimpen ke folder connector `mcp-github` (config + token, self-managed).

**Enable:** `mcpclient` spawn proses (`exec.Command(command, args...)` + env) → JSON-RPC stdio:
`initialize` → `tools/list` → tiap tool di-`tools.RegisterDynamic` sebagai `mcp_github_<tool>` (Capability
`mcp:github`, owner-approved). Proses persisten (1 per connector, reap pas disable/crash → respawn).

**Agent pake:** agent `tool_search "github issue"` → nemu `mcp_github_create_issue` (di registry) → `tool.run`
→ `SandboxRunV3` → tool.Run() → `mcpclient` kirim `tools/call` ke proses github-mcp → balikin hasil ke agent.

**Uncheck (per-agent):** setting agent X → MCP section → uncheck "github" → ditulis ke state.db X (exclusion)
→ `tool_search`/`tool.specs` agent X nge-skip tool `mcp_github_*`. Agent lain ga kena.

---

## 🗺️ PHASES (8-step per phase: build→review→test→lock→changelog→push)

### ✅ Phase 1 — `internal/mcpclient`: MCP client (stdio) + dogfood test (DONE 06-06)
Spawn server MCP via exec, JSON-RPC stdio (`initialize`/`tools/list`/`tools/call`/close). **Test DOGFOOD:**
colok ke `bin/flowork-mcp` sendiri (lokal, no dependency luar) → list tools → call `chat` → dapet reply.

### ✅ Phase 2 — MCP connector registry (`kind:mcp`) + tool bridge (DONE 06-06)
Extend `internal/connections`: install MCP connector (config command/args/env+token di folder sendiri),
enable → spawn + `RegisterDynamic` tiap tool, disable → Unregister + reap. Test: install→enable→tool muncul
di registry→run lewat tool.run.

### ✅ Phase 3 — per-agent uncheck + GUI 2 kategori (DONE 06-06)
Exclusion connector MCP per-agent (state.db) + `tool_search`/`specs` hormatin. GUI Connections 2 kategori
(Channels + MCP) + setting agent section MCP (checklist). i18n.

---

## 🔒 KEAMANAN
- MCP connector = **owner-install + owner-token** (GrantOwner). AI ga bisa nambah/aktifin sendiri.
- Tool MCP capability `mcp:<conn>` → consent owner. Eksekusi lewat `SandboxRunV3` (gate existing).
- Token MCP di folder connector sendiri (0600), di-mask di API. Proses MCP reap bersih pas disable/uninstall.

## 📌 DEFER (post-MVP)
- Transport HTTP/SSE (stdio dulu — standar + cocok github/fs MCP).
- Per-TOOL uncheck (per-connector dulu cukup 90% kasus).
- Auto-respawn policy + resource cap per proses MCP.
