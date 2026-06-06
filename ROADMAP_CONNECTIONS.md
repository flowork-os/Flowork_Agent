# 🔌 ROADMAP — "Connections" (sistem connector universal)

> **Status:** AKTIF (mulai 2026-06-06). Living doc. Ditandai ✅ per phase yang SELESAI+TEST+PUSH.
>
> **Buat AI eksternal yang audit kerjaan ini:** baca section "KENAPA arsitektur ini" dulu — semua
> keputusan teknis di sini DISENGAJA, ada alasannya, jangan dikira asal. Owner (Mr.Dev) nentuin GOL;
> arsitektur teknis diputusin AI. Kalau lo mau ngubah, ngerti dulu trade-off yang udah dipikirin.

---

## 🎯 GOL (dari owner — FIXED, jangan diubah)

Nambah surface I/O baru (Discord, email, CLI, schedule, MCP, dst) ke Flowork, dengan syarat:
**multi-OS (Windows/Linux/Mac) · portable · plug-and-play · terisolasi.** Prinsip inti:
**saat 1 connector error, kita HANYA perbaiki folder connector itu — ga nyentuh file lain / connector lain / kernel.**

Owner lumpuh, ngetik mahal → eksekusi **full-autonomous**: owner kasih arah, AI putusin & selesaikan.

---

## 🧠 KENAPA arsitektur ini (rationale — WAJIB dibaca auditor)

Tiap keputusan di bawah dipilih karena **satu lensa: gol di atas**. Bukan selera.

### 1. Connector = **WASM (wazero)**, BUKAN binary native per-OS
- **Kenapa:** wazero itu runtime wasm pure-Go, **nol cgo**. Satu file `agent.wasm` jalan IDENTIK di
  Windows/Linux/Mac tanpa rebuild, tanpa DLL/.so, tanpa beda spawn-proses antar-OS.
- **Alternatif yang DITOLAK:** "native sidecar subprocess" (proses OS terpisah biar bisa buka socket).
  Ditolak karena butuh build 3 OS per-connector + manajemen proses beda-beda tiap OS = **lawan gol multi-OS**.
- **Bukti pola:** `telegram-channel.fwagent` udah jalan persis model ini.

### 2. Transport = **HTTP doang** (via `host_net_fetch`)
- **Kenapa:** modul wasm cuma punya 1 primitif jaringan (`host_net_fetch` = HTTP). Hampir SEMUA platform
  modern punya HTTP API resmi → kita pakai itu, dan **sengaja hindari protokol socket mentah**
  (IMAP/SMTP, Discord-Gateway-WS). Email → Gmail/Graph/SendGrid API (bukan IMAP). WhatsApp → Cloud API Meta.
- **Hasil:** semua connector tetep wasm murni → tetep portable. Nol cap baru di kernel (kernel tetep beku).

### 3. Input = **POLLING** (default), webhook opsional
- **Kenapa:** desktop di Win/Mac/Linux biasanya di belakang NAT → **ga bisa nerima webhook** dari internet.
  Long-poll (tarik update) jalan di belakang NAT apa pun. Webhook (`/api/kernel/webhook/<id>`, udah ada)
  cuma buat deploy server.

### 4. State per-connector di **FOLDER CONNECTOR SENDIRI**, BUKAN tabel pusat
- **Kenapa (INI INTI prinsip "1 error = 1 folder"):** kalau status enable/config tiap connector ditaro di
  satu tabel `channels` bersama, maka 1 baris korup / 1 bug schema = **nyentuh semua connector**. Itu
  ngelanggar isolasi. Jadi: config + enabled-state tiap connector di **loket store-nya sendiri**
  (`<id>.fwagent/loket.db`). **Uninstall = hapus folder → semua state ilang, NOL cleanup pusat.**
- **Pengecualian SECRET (token):** token sensitif (kontrak §F: "kredensial = infra shared"). Token disimpen
  di vault terenkripsi pusat (`floworkdb secrets`, pola `settingsapi` yg udah proven) + di-inject sebagai
  env pas connector boot. **Non-secret config (target agent, enabled, allowed-users) tetep per-folder.**
  Split ini: isolasi (config per-folder) + keamanan (secret terenkripsi terpusat) — dua-duanya kepegang.

### 5. Install/uninstall lewat **gerbang `.fwpack` yang UDAH ADA**
- **Kenapa:** `installPluginPack` udah dispatch by-kind (tool/slash/scanner). Tinggal tambah `case "channel"`.
  Nol mekanisme install baru. Hot-load via fsnotify watcher yg udah ada → drop folder = colok.

### 6. Backend di package **TERISOLASI `internal/connections/`**
- **Kenapa:** logika manage connector ga boleh nyampur ke `agentmgr`/`main`. Package sendiri = kalau sistem
  connections bug, blast-radius-nya 1 package. Pola sama persis `internal/scanapi` (scanner pack terisolasi).

### 7. Isolasi ke-backing MEKANISME NYATA (bukan harapan)
- Kernel `dispatcher.go` punya `recover()`: connector panic → jadi error Result, **kernel + connector lain
  TETEP IDUP** (ini chokepoint yg udah di-audit security 2026-06-06). + wazero sandbox per-folder.
- → Discord-connector crash → Telegram/Email/kernel AMAN. Benerin folder `discord` doang. **Arsitektural.**

### 8. MCP dipecah 2 (jangan ketuker)
- **MCP-server** = client luar (Claude Desktop) pake Flowork → ini **connector** (protocol channel).
- **MCP-client** = Flowork makan tool MCP luar → ini **tool-source** (nyolok `tool.specs/tool.run`), BUKAN connector.

---

## 🗺️ PHASES (8-step per phase: build→review→test→lock→changelog→push→next)

### ✅ Phase 1 — Backend registry `internal/connections/` + gerbang `kind:channel` (DONE 06-06, pushed 250012a)
Package terisolasi: `List` (scan AgentsDir kind:channel) · `InstallChannelPack` (extract wasm, anti
zip-slip + refuse GrantOwner caps + file-cap) · `SetEnabled` (marker file di folder) · `Uninstall`
(hapus folder) · `GetConfig/SetConfig` (token self-managed di `connector.json` folder, 0600, masked).
Endpoint `/api/connections{,/toggle,/config,/uninstall}`. Dispatch `case "channel"` di `plugin_handler.go`.
**Test:** 6 go test (lifecycle/reject-nonchannel/reject-nowasm/reject-ownercaps/zip-slip/id-traversal). LOCKED.

### ✅ Phase 2 — CLI connector + Connector SDK template (DONE 06-06, pushed d2c773a)
`cmd/flowork-connect/` — CLI connector HOST-SIDE (terminal ga bisa di-wasm), dumb-pipe via `/api/kernel/rpc`,
self-managed config, multi-OS (cross-compile win/mac OK), **harness QC**. TEST LIVE: mr-flow-next jawab LLM.
`templates/connector-template/` — core dumb-pipe siap + 3 `TODO(connector)` (config/poll/send) + README.
Build `GOOS=wasip1` wasm OK. CLI LOCKED; template sengaja ga di-lock (buat dicopas).

### ✅ Phase 3 — GUI tab "Connections" (DONE 06-06)
Galeri Jarvis-HUD (`web/tabs/connections.js`): list + status (LIVE/IDLE) + toggle + config (token+target,
disclaim disimpen di folder connector) + uninstall + install drop `.fwpack`. Daftar di `index.html` nav +
`ACTIVE_TABS` + i18n DOMAINS. i18n en+id (`connections.json`). **Test:** handler httptest (list/config-mask/
toggle/uninstall) + route live (401=wired) + JSON valid + rebuild/restart OK.

### ⏳ Phase 4 — Connector platform pertama (Discord/email/…) — READY, butuh pilihan + kredensial owner
Fondasi (registry+gerbang+CLI+template+GUI) UDAH lengkap & tested. Connector platform real = copas
`connector-template` → isi 3 TODO (config/poll/send) pakai HTTP API platform → build wasm → install.
**Butuh owner:** pilih platform (Discord/email/WhatsApp) + token (ga bisa di-test live tanpa kredensial).
telegram-channel + CLI connector udah jadi 2 bukti pola jalan. Idle-tanpa-token + isolasi terjamin by-design.

---

## 📌 BELUM DIPUTUS (nunggu, ga blokir Phase 1-2)
- Prioritas surface pertama (Discord dulu? Email dulu?).
- Detail credential-vault GUI (Phase 3) + reply-routing registry (push proaktif `bus.send(to:"owner")`).
- 2 PR multi-OS di luar connector: sweep hardcode `"/"` (`sec29_35.go`) + guardian `chattr` Linux-only (per-OS).
