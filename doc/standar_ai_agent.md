# Standar AI Agent — Flowork

> Dokumen ini adalah kontrak. Setiap AI agent baru, setiap perubahan kode
> yang menyentuh agent, dan setiap review wajib lewat dokumen ini dulu.
> Kalau ada konflik antara dokumen ini dan kode → kode yang salah.

---

## 1. Filosofi (kenapa kita begini)

Flowork itu kumpulan AI (kita sebut warga) yang hidup di satu microkernel.
Tiap warga punya kepribadian, tugas, dan kebutuhan sendiri. Mr.Flow handle
Telegram, agent lain mungkin handle YouTube, scrap data, code, dsb.

Karena tiap warga beda fungsi → beda kebutuhan setingan → beda data →
**wajib terisolasi**. Bocor satu, bukan cuma agent itu yang rusak, tapi
warga lain ikut kebawa. Itu yang harus dicegah.

### Prinsip yang kita anut

| Prinsip | Artinya untuk agent |
|---|---|
| **Portable** | Agent harus jalan tanpa setup global. Semua state milik dia ikut folder dia. Pindah folder = pindah komputer = tetap jalan. |
| **Nano modular** | Satu agent = satu fungsi jelas. Jangan campur tugas. Mau tambah fungsi baru → bikin agent baru, jangan tempel ke yang lama. |
| **Plug and play** | User taruh agent baru → kernel pickup otomatis (hot-reload). User cabut agent → bersih total, ngga ada sisa. |
| **Multi-OS** | Code harus jalan di Windows, Linux, macOS. Path pakai `filepath.Join`, jangan hardcode `/`. Hindari shell-specific tool. |
| **White label** | Branding bukan domain agent. Semua label/icon/copy bisa diganti tanpa nyentuh logic. |
| **Deterministik dulu** | Logika kritis (routing, kategori, gating) di KODE, bukan LLM. "deterministik = kuat, LLM lemah = rapuh." Detail: **section 12.0**. |
| **Belajar, jangan diapalin** | Knowledge & koreksi di brain (INGEST, updatable), bukan hardcode/over-prompt/di-train. Agent makin pinter dari mistakes+karma (sistem imun), bukan dari prompt makin gemuk. Detail: **section 12**. |
| **Anti-halu by design** | Tiap agent yang mikir/dispatch ke-cover antibody injection di gateway (mistakes karma-ranked, MAX 3, di-inject deterministik). Detail: **section 12.4**. |

### Yang dilarang keras

- **Hardcode** apapun yang user/operator bisa atur. Prompt, persona,
  endpoint, token, chat ID, schedule, skill — **wajib di database**,
  bukan di string literal Go/JS. Hardcode = audit fail.
- **Cross-agent leak**. Agent A ngga boleh tau apapun soal agent B
  kecuali lewat workspace bersama yang eksplisit.
- **State global** yang ngga ke-track. Setiap state harus punya rumah
  (DB per-agent atau workspace per-agent).
- **Ubah/pindah/cabut jalur kritis** (routing deterministik, antibody hook, pipa imun)
  tanpa izin eksplisit Mr.Dev. Dijaga `wiring_invariant_auditor` → CRITICAL kalau putus.
  Akar masalah lama: "AI suka rubah jalur" → arsitektur keputus → halu balik. Detail: **section 12.5**.
- **Gantungin logika kritis ke LLM** (ngarep model manggil tool / inget aturan sendiri).
  Paksa lewat kode/injeksi deterministik. Detail: **section 12.0 & 12.4**.

---

## 2. Standar Agent — 9 wajib terisolasi

Setiap agent **wajib** punya 9 hal ini, dan **wajib terisolasi** per agent.
Implementasi semua hidup di **folder agent itu sendiri** —
`agents/<id>/` (source) dengan symlink ke
`~/.flowork/agents/<id>.fwagent/workspace/` (runtime).

| # | Komponen | Wajib | Storage (HARDCODED) | Dilihat agent sebagai |
|---|---|---|---|---|
| 1 | **Prompt** | ✅ isolated | `state.db` → `kv` (k=`prompt`) | env `FLOWORK_AGENT_CONFIG.prompt` |
| 2 | **Schedule** | ✅ isolated | `state.db` → `schedules` (id, cron, task) | env `FLOWORK_AGENT_CONFIG.schedule[]` |
| 3 | **Tools** | ✅ isolated | `state.db` → `tools` (name) — **default centang semua** kalau fresh | env `FLOWORK_AGENT_CONFIG.tools[]` |
| 4 | **Skills** | ✅ isolated | `state.db` → `skills` (id, trigger, instructions) | env `FLOWORK_AGENT_CONFIG.skills[]` |
| 5 | **Workspace privat** | ✅ isolated | folder `agents/<id>/workspace/` (HARDCODED nama `workspace`) | WASI mount `/workspace` |
| 6 | **Settings** | ✅ isolated | `state.db` → `kv` (router_url, router_model) + `secrets` (semua API key) | env `FLOWORK_AGENT_CONFIG.router` + tiap secret jadi env var (`TELEGRAM_BOT_TOKEN`, `GOOGLE_API_KEY`, dst) |
| 7 | **SQLite DB** | ✅ isolated | `agents/<id>/workspace/state.db` (HARDCODED) | path `/workspace/state.db` |
| 8 | **Popup UI** | ✅ isolated | `agents/<id>/ui/setting.html` (per-agent custom) | rendered di kartu klik ⚙️ Setting |
| 9 | **Workspace shared** | ✅ semi-isolated | folder `<root>/workspace/` (HARDCODED di root project) | WASI mount `/shared` |

### Penjelasan per komponen

#### 1. Prompt (system prompt / persona)
- Lokasi DB: tabel `kv` dengan key `prompt`
- Inject ke agent: dalam `FLOWORK_AGENT_CONFIG.prompt`
- **Dilarang** di-hardcode di main.go. Default value boleh ada di code,
  tapi cuma sebagai fallback kalau DB kosong (lihat `mr-flow/main.go:defaultPersona`).
- Edit via popup section 1.

#### 2. Schedule (tugas berkala)
- Lokasi DB: tabel `schedules (id, cron, task, order_idx)`
- Format: cron 5-kolom (`menit jam tgl bulan hari-minggu`), recurring atau one-shot
- Edit via popup section 2: tambah/hapus baris bebas.

#### 3. Tools (capability toggle)
- Lokasi DB: tabel `tools (name)` — presence = enabled
- Predefined: `telegram`, `router`, `kv`, `fs`, `net`. Agent baru
  bisa nambah nama lain — kontrak: nama sama dengan flag yang agent baca.
- **Default centang semua** kalau popup di-buka pertama kali (`cfg` empty).
  Setelah save sekali apa pun, respect pilihan user (boleh uncheck semua).
- Edit via popup section 3: checkbox.

#### 4. Skills (procedure / behavior reusable)
- Lokasi DB: tabel `skills (id, trigger, instructions, order_idx)`
- Trigger = sinyal (mis. `/sum`, atau keyword). Instructions = mini-prompt
  yang ditempel ke system message saat LLM call.
- Edit via popup section 4.

#### 5. Workspace privat (folder kerja agent — eksklusif)
- **HARDCODED**: `agents/<id>/workspace/` (di source folder agent).
- Mount guest: `/workspace` (selalu, no cap gate).
- **Cuma agent itu yang bisa akses**. Agent lain tidak punya mount ke folder ini.
- Agent bebas bikin file/folder apapun di sini (user-data, cache state, dst).
- File-file yang ada by default:
  - `state.db` — SQLite (lihat section 7)
- Standar nama folder = `workspace` (lowercase). Tidak boleh diganti.

#### 5b. Workspace shared (folder bersama lintas agent)
- **HARDCODED**: `<project-root>/workspace/` (di root, sejajar `agents/`).
- Mount guest: `/shared` (selalu, no cap gate).
- Struktur per-agent **auto-create saat boot** kernel:
  ```
  <root>/workspace/
  ├── _global/                    ← bahan bareng lintas agent (anyone read/write)
  ├── _index.json                 ← (TODO) registry tools yang agent bikin
  ├── <agent-id-A>/
  │   ├── tools/                  ← script/tool yang agent bikin (.py, .sh, .go)
  │   │                              ← bisa di-load agent lain (real sharing)
  │   ├── job/                    ← output kerjaan (hasil scrape/process)
  │   ├── document/               ← markdown, notes, report
  │   ├── media/                  ← audio, video, image
  │   ├── cache/                  ← cache temporary (agent boleh hapus sendiri)
  │   └── log/                    ← log per-agent
  └── <agent-id-B>/
      └── (struktur sama)
  ```
- 6 subfolder standar: **tools, job, document, media, cache, log**.
- Agent bisa baca folder agent lain di `/shared/other_id/...` — untuk
  collaboration (mis. pakai tool yang agent lain bikin).
- Agent bisa tulis dimanapun di `/shared/` (no enforcement filesystem,
  trust + audit). Konvensi: agent cuma tulis di folder dia sendiri.
- **Tools generation pattern**: kalau agent butuh tool yang ngga ada,
  generate via LLM → simpan `/shared/<my_id>/tools/<name>.py` →
  execute via host_exec_run. Tool ke-cache untuk next time.

#### 6. Settings (router LLM + credentials API)
- Router URL + model: tabel `kv` (key `router_url`, `router_model`)
- API keys (Telegram bot token, chat IDs, Google API, YouTube API, dst):
  tabel `secrets (k, v)`. Tiap row jadi env var di agent runtime.
- **Wajib mandiri per agent**. Mr.Flow boleh pakai router A, agent lain
  pakai router B. Token Telegram per agent (kalau dua agent pakai bot
  yang sama, masing-masing simpan token sendiri).
- Edit via popup section 6.

#### 7. SQLite Database (state runtime + config)
- **HARDCODED**: `agents/<id>/workspace/state.db` (di dalam workspace
  privat agent). Tidak butuh symlink — workspace folder mount langsung.
- Agent buka via path konstanta `/workspace/state.db`.
- Schema saat ini: `kv`, `schedules`, `tools`, `skills`, `secrets`, `meta`.
- Agent boleh bikin tabel sendiri buat state user-data (mis. tabel
  `messages` untuk chat history). Tabel default jangan diutak-atik
  (dipakai kernel buat inject config).
- Reset DB lewat tombol di popup section 7. Reset = file dihapus +
  retouch kosong → semua config hilang permanen.

#### 8. Popup UI (per-agent setting form — declarative schema)
- Popup = **7 section standar universal** (Prompt / Schedule / Tools /
  Skills / Workspace / Settings / Database) + **extra section dari agent
  via schema declarative**.
- Agent declare extra section di `manifest.json.ui_schema.sections[]`
  (lihat section 7 dokumen ini untuk format). Renderer otomatis bikin
  field — agent ngga perlu nulis HTML/JS.
- Kalau butuh widget yang lebih kompleks (preview chart, file picker,
  dll), agent boleh ship `agents/<id>/ui/setting.html` sebagai opt-in.
  Saat ini fitur full-HTML belum implement; schema declarative dulu
  yang cover 95% kasus.
- Kartu agent (di tab AI Agent) **wajib punya**:
  - **Switch enable/disable** — toggle on/off tanpa hapus agent ✅
  - **Tombol Setting** — buka popup (7 standar + schema extra) ✅
  - **Tombol Download** — bundle folder agent jadi `.fwagent.zip`
    termasuk `workspace/state.db` (semua setting), source code, dll ✅

#### 9. Akses filesystem (sandboxing)
- Agent biasa: **hanya boleh** write/read/delete di:
  - `/workspace/` — private, eksklusif (cuma agent ini)
  - `/shared/` — bareng lintas agent (konvensi: tulis di `/shared/<my_id>/`)
- Akses ke filesystem host lain di luar mount → blocked oleh WASI
  preopen (wazero `WithDirMount`).
- **Pengecualian: agent privileged** (lihat section 4).

---

## 3. Lokasi file & layout (cheat sheet — HARDCODED konvensi)

```
Flowork_Agent/                       ← project root
├── agents/                          ← source tiap agent
│   └── <id>/
│       ├── manifest.json            ← metadata (id, kind, caps, entry)
│       ├── main.go                  ← source Go (compile ke WASI)
│       ├── workspace/               ← workspace privat agent (HARDCODED)
│       │   └── state.db             ← SQLite per-agent (HARDCODED)
│       ├── ui/
│       │   └── setting.html         ← popup custom per agent (TODO)
│       └── i18n/
│           ├── id.json              ← optional bahasa lokal
│           └── en.json
├── workspace/                       ← SHARED workspace di root (HARDCODED)
│   ├── _global/                     ← bahan bareng lintas-agent
│   ├── <agent-id-A>/
│   │   ├── tools/                   ← script/tool dibikin agent (shareable)
│   │   ├── job/                     ← output kerjaan
│   │   ├── document/                ← markdown, notes, report
│   │   ├── media/                   ← audio, video, image
│   │   ├── cache/                   ← cache temporary
│   │   └── log/                     ← log per-agent
│   └── <agent-id-B>/
│       └── (struktur sama, 6 subfolder auto-create)
├── scripts/build-agent.sh           ← compile source → .wasm + stage
└── ...

~/.flowork/                          ← runtime (di-manage kernel, fallback only)
└── agents/
    └── <id>.fwagent/
        ├── manifest.json            ← copy dari source (atau hasil unzip)
        └── agent.wasm               ← hasil compile TinyGo
```

**Aturan path (HARDCODED konvensi):**

| Apa | Host path (source-backed) | Guest path (dilihat agent) |
|---|---|---|
| Workspace privat | `agents/<id>/workspace/` | `/workspace` |
| SQLite DB | `agents/<id>/workspace/state.db` | `/workspace/state.db` |
| Workspace shared (root) | `<project>/workspace/` | `/shared` |
| Folder agent di shared | `<project>/workspace/<id>/` | `/shared/<id>/` |
| Tools shareable | `<project>/workspace/<id>/tools/` | `/shared/<id>/tools/` |
| Bahan bareng | `<project>/workspace/_global/` | `/shared/_global/` |

**Catatan operasional:**
- Source-backed (folder `agents/<id>/` ada): path di atas dipakai apa adanya.
- Installed-from-zip (no source): fallback ke `~/.flowork/agents/<id>.fwagent/workspace/`.
- Hapus source = hapus agent permanen (state DB di workspace ikut hilang).
- Download agent dari UI = zip seluruh `agents/<id>/` (termasuk
  `workspace/state.db` + semua user-data) → portable ke komputer lain.

---

## 4. Agent privileged (pengecualian akses filesystem)

Sebagian kecil agent punya tugas yang memang butuh akses ke filesystem
host di luar workspace. Contoh:

- **Team Coder** — baca/tulis file project nyata
- **Operator PC** — eksekusi command, manipulasi file system
- **Backup Agent** — copy file lintas folder

Agent kategori ini:
1. **Wajib declare capability spesifik** di manifest
   (mis. `fs:host:/home/mrflow/Projects/`, `exec:shell`, `fs:host:**`).
2. **Wajib review manual** sebelum capability di-approve oleh broker.
3. **Wajib audit log** — setiap akses tercatat (TODO).
4. Tetap **isolated** dari agent lain (tools, prompt, secrets, schedule,
   skills, settings, DB, popup UI tetap di folder masing-masing).
   Yang dilonggarkan cuma filesystem mount.

Default broker policy: **deny by default**, approve eksplisit.
Owner yang setuju, owner yang nanggung.

---

## 5. Workflow standar (state machine)

```
[create]   user upload .fwagent.zip atau bikin folder di agents/<id>/
   ↓
[scan]     kernel scan staged → validate manifest → reject kalau invalid
   ↓
[boot]     kernel ensure workspace + DB + symlink, open store, migrate
           config.json kalau ada (legacy), build env, instantiate WASI
   ↓
[daemon]   kalau manifest declare `boot` di exposes_rpc, kernel auto-call
   ↓ (running long-poll / scheduler / idle)
   ↓
[config]   user save via popup → ConfigHandler → store.Save → callback
           host.ReloadAgent → unload + reload + auto-boot (env baru)
   ↓
[disable]  user toggle switch off → POST /api/agents/toggle?disabled=1
           → store.SetDisabled(true) → host.ReloadAgent → kernel unload
           wasm instance + skip daemon. Folder + state utuh.
[enable]   user toggle switch on → POST /api/agents/toggle?disabled=0
           → instance re-instantiate + auto-boot daemon dengan env baru.
[remove]   user klik hapus → folder agents/<id>/ + staged dihapus
[download] user klik download → GET /api/agents/download?id=<id>
           → zip seluruh agents/<id>/ (include workspace/state.db,
           manifest, source code) → .fwagent.zip portable
```

---

## 6. Kontrak env yang kernel inject ke agent

Setiap agent saat di-instantiate dapet env berikut. Bisa diandalkan.

| Env | Isi | Kapan ada |
|---|---|---|
| `FLOWORK_AGENT_ID` | id agent (mis. `mr-flow`) | selalu |
| `FLOWORK_AGENT_WORKSPACE` | `/workspace` (HARDCODED) | selalu |
| `FLOWORK_AGENT_DB` | `/workspace/state.db` (HARDCODED) | selalu |
| `FLOWORK_SHARED_WORKSPACE` | `/shared` (HARDCODED, root project workspace) | selalu (no cap gate) |
| `FLOWORK_AGENT_CONFIG` | JSON utuh: `{prompt, router, schedule[], tools[], skills[]}` | selalu (kalau DB punya isi) |
| `<KEY>` dari `secrets` | string value | tiap row `secrets` di-expand jadi env var (mis. `TELEGRAM_BOT_TOKEN`, `GOOGLE_API_KEY`) |

**Catatan**: path workspace + DB + shared sudah HARDCODED konstanta —
env disediakan untuk informational/portability. Agent boleh tulis
langsung `const WorkspaceDB = "/workspace/state.db"` (lihat mr-flow
main.go) tanpa baca env.

Penamaan secret = penamaan env. Kalau agent baca `os.Getenv("FOO_API_KEY")`,
user simpan secret dengan key `FOO_API_KEY` di popup.

---

## 7. Popup customization — `manifest.json.ui_schema`

Tiap agent boleh declare extra section di popup setting tanpa nulis
HTML/JS. Format declarative di manifest:

```json
{
  "ui_schema": {
    "sections": [
      {
        "id": "telegram",                    // unique slug
        "icon": "✈️",                         // emoji prefix
        "title": "Telegram Bot",             // header label
        "description": "Optional sub-text",
        "fields": [
          {
            "key": "TELEGRAM_BOT_TOKEN",     // = env var name kalau storage=secrets
            "label": "Bot Token",            // user-facing label
            "type": "password",              // widget type
            "storage": "secrets",            // tabel di state.db
            "required": true,                // visual hint *
            "placeholder": "123456:ABC...",
            "help": "Penjelasan tooltip"
          }
        ]
      }
    ]
  }
}
```

### Field types yang renderer support

| `type` | Widget | Use case |
|---|---|---|
| `text` | input text | nama, ID, URL, key |
| `password` | input password (masked) | secret, token, API key |
| `textarea` | multi-line text | prompt panjang, JSON config |
| `number` | input number | port, timeout, batch size |
| `select` | dropdown (butuh `options: [{value, label}]`) | enum (mis. region, model) |
| `checkbox` | toggle | boolean flag |
| `json` | textarea + monospace | nested config, list complex |

### Storage destinations

| `storage` | Tabel | Dibaca agent sebagai | Note |
|---|---|---|---|
| `secrets` (default) | `secrets` | env var (key = field.key) | buat API key, token, credential |
| `kv` | `kv` | masuk `FLOWORK_AGENT_CONFIG.kv[key]` | buat config non-sensitive |
| `meta` | `meta` | masuk `FLOWORK_AGENT_CONFIG.meta[key]` | buat flag/state internal |

### Aturan declare schema

- Field `key` wajib unique per agent.
- Field `key` di `storage=secrets` jadi env var literal — agen baca
  via `os.Getenv("FOO_API_KEY")`. Penamaan **wajib UPPER_SNAKE_CASE**.
- Reserved keys (jangan dipakai di schema):
  - `kv`: `prompt`, `router_url`, `router_model`
  - `meta`: `disabled`, `schema_version`
- Section auto-numbered mulai dari **8** (setelah 7 standar). Bisa ada
  banyak section per agent.
- Field generic Credentials list di section 6 (Settings) tetap ada
  sebagai **escape hatch** untuk power user nambahin secret yang
  ngga ke-declare di schema.

### Contoh: mr-flow

```json
"ui_schema": {
  "sections": [{
    "id": "telegram",
    "icon": "✈️",
    "title": "Telegram Bot",
    "description": "Bot token + chat IDs.",
    "fields": [
      { "key": "TELEGRAM_BOT_TOKEN", "label": "Bot Token",
        "type": "password", "storage": "secrets", "required": true,
        "placeholder": "123456:ABC...",
        "help": "Format: <bot_id>:<secret>. Dari @BotFather." },
      { "key": "TELEGRAM_ALLOWED_CHATS", "label": "Allowed Chat IDs",
        "type": "text", "storage": "secrets", "required": true,
        "placeholder": "123456789, -1001234567890",
        "help": "Pisahkan koma. Group = negative." }
    ]
  }]
}
```

Agent code (mr-flow `main.go`):
```go
token := os.Getenv("TELEGRAM_BOT_TOKEN")
chats := os.Getenv("TELEGRAM_ALLOWED_CHATS")
```

Tidak perlu parse JSON config — kernel sudah expand secrets jadi env.

---

## 8. Checklist sebelum merge

Setiap PR yang nyentuh agent infrastruktur **wajib** centang ini:

- [ ] Apakah ada string yang di-hardcode yang seharusnya di DB? → pindah.
- [ ] Apakah konfig agent leak ke agent lain? → audit isolation.
- [ ] Apakah file/folder baru di-buat di luar workspace agent? → fix.
- [ ] Apakah path pakai `filepath.Join`, bukan hardcoded `/`? → multi-OS.
- [ ] Apakah popup UI custom per agent atau masih generic? → pindah ke `agents/<id>/ui/`.
- [ ] Apakah kartu punya switch enable/disable + tombol download? → tambah.
- [ ] Apakah download zip include `state.db` + `workspace/`? → verify.
- [ ] Apakah dictionary i18n di-update (no inline copy)? → update.
- [ ] Apakah ada test? (TODO: framework test belum ada)

---

## 9. TODO (gap antara standar dan kode sekarang)

Status implementasi sampai 2026-05-29 (post-hardcode refactor):

| # | Standar | Status |
|---|---|---|
| 1 | Prompt isolated | ✅ implement (DB `kv.prompt`) |
| 2 | Schedule isolated | ✅ implement (DB `schedules`) — scheduler runtime: ❌ belum |
| 3 | Tools isolated + default centang semua | ✅ implement (DB `tools` + frontend `isFresh ? TOOL_FLAGS : []`) |
| 4 | Skills isolated | ✅ implement (DB `skills`) |
| 5 | Workspace privat hardcoded `/workspace` | ✅ implement (mount langsung, no symlink) |
| 5b | Workspace shared hardcoded di root + 6 subfolder | ✅ implement (`workspace/<id>/{tools,job,document,media,cache,log}` auto-create) |
| 6 | Settings isolated | ✅ implement (DB `kv` + `secrets` → env) |
| 7 | SQLite per agent hardcoded `/workspace/state.db` | ✅ implement (no symlink dance) |
| 8 | Popup custom per agent (declarative `manifest.ui_schema`) | ✅ implement (renderer 7 type field, storage routing secrets/kv/meta) |
| 8 | Popup full-HTML opt-in (`agents/<id>/ui/setting.html`) | ❌ design only — schema declarative dulu cover 95% kasus |
| 8 | Switch enable/disable di kartu | ✅ implement (`meta.disabled` di DB, `/api/agents/toggle`, kernel skip-daemon + UI switch) |
| 8 | Download agent → zip (include workspace) | ✅ implement (zip dari source folder, include `workspace/state.db` + semua user-data) |
| 9 | Filesystem scoped via WASI | ✅ implement (kernel mount per-agent + shared root) |
| 9 | Tool generation pattern (LLM→save→exec) | ❌ design only |
| 9 | `_index.json` registry tools | ❌ belum |
| 9 | Privileged agent class | ❌ belum (broker policy belum ada review-manual) |
| - | Multi-OS path | ⚠️ pakai `filepath.Join`, belum test Windows |
| - | Plug-and-play (hot-reload) | ✅ implement via fsnotify + `host.ReloadAgent` |
| - | White label | ⚠️ branding "Flowork" masih di code, belum theme-able |

Prioritas urut: **8** (popup custom + switch + download lengkap dari source) >
**2** (scheduler runtime) > **9** (tool gen + privileged) > white label.

---

## 11. Prompt Budget (anti over-prompt — anti-halu)

> Ditambahkan 2026-05-29 setelah review risk over-prompt dari iterasi sebelumnya. Mr.Dev cerita: di iterasi lama, AI halu karena over-prompt — apalagi LLM lokal yang context kecil.

### Filosofi

**Over-prompt = silent killer.** LLM dapet system prompt 8000 token → kebanyakan instruksi → halu, lupa task asli, ngarang fakta. Local LLM (Qwen 7B Q4) context kecil (~8K-32K) — kalau persona alone makan 4K, jawaban berkualitas mustahil.

**Prinsip:** **on-demand fetch via tool call > always-on injection.**

### Budget allocation (target persentase context window)

| Layer | Always-on (auto-inject) | On-demand (tool call) |
|---|---|---|
| Persona (cfg.prompt) | ≤ 500 tok (~2000 char) | — |
| Skills | **MAX 3** aktif, ≤ 300 tok | `skill_search` untuk sisa |
| Tools | **5 core**: read/write/bash/brain_search/telegram_send | `tool_search` untuk sisa 23+ |
| Brain drawers (router enrich) | **top-3 relevant**, ≤ 1000 tok | `brain_search` untuk deep query |
| Episodic interactions | **0 auto** (always tool call) | `memory_get` |
| Mistakes / antibody | **top-3 relevant only**, ≤ 500 tok | `brain_search type=antibody` |
| Facts | **0 auto** (always tool call) | `fact_recall` |
| Codemap nodes | **0 auto** (always tool call) | `codemap_*` tools, return summary max 10 nodes |
| **Total budget** | **≤ 30% context window** | — |

### Implementation contract

Setiap section yang inject ke system prompt **WAJIB**:

1. **Declare budget constant** di awal package:
   ```go
   const (
       maxXxxItems     = 3
       maxXxxCharsItem = 300
       maxXxxTotal     = 1000
   )
   ```
2. **Hard cap di end** — last-ditch defense:
   ```go
   if len(s) > maxXxxTotal {
       s = s[:maxXxxTotal] + "…[truncated]"
   }
   ```
3. **Truncation log** ke `decisions` (Agent section 3) supaya owner bisa monitor.

### Compact tool — emergency valve

Section 11 tier1 tool `compact_context.go` (sudah di referensifile). Trigger:
- Saat context > 70% window → auto-summarize chat history jadi drawer
- Clear chat → resume dengan drawer summary di awal

### Anti-pattern (DILARANG)

❌ **Auto-inject ALL skills** ke persona (bug awal mr-flow — sudah di-fix 2026-05-29)
❌ **Dump full tool catalog** ke system prompt
❌ **Brain retrieval tanpa top-K filter**
❌ **Stuff episodic history** ke prompt biar warga "inget"
❌ **Mistakes list growing** (kalau 50 mistakes, 50-list di-inject = bloat)
❌ **Codemap full graph** dump

### Pattern referensi yang OK

✅ Persona base (cfg.prompt) ≤ 500 tok
✅ Skills auto-inject MAX 3, sisanya teaser `"…+N skill lain (panggil skill_search)"`
✅ Tool description on-demand by name (warga panggil `tool_describe` ke registry)
✅ Brain retrieval: top-3 drawer, semantic match query, dengan provenance tag
✅ Compact tool tersedia tapi tidak auto-fire (warga sendiri yang call)

### ⭐ Sub-section penting: Inter-Warga Communication WAJIB Self-Contained

Kalau warga A delegate task ke warga B (RPC, mesh, atau intra-host call), **WAJIB pakai self-contained prompt** — pattern dari Architect Agent Blueprint, lihat roadmap Bagian 9 Section 35.

**Aturan WAJIB:**
1. Prompt delegate WAJIB punya: Task, Context, Constraints, Acceptance Criteria, Deliverable
2. Prompt WAJIB lulus length cap (≤ 2000 char ≈ 500 tok)
3. Prompt **DILARANG** referensi "history sebelumnya" — pattern detector reject phrase `"see history"`, `"prior conversation"`, `"as we discussed"`, dst.
4. Warga B execute dengan **COLD context** — ngga ada chat history dari A

**❌ Anti-pattern (DILARANG):**
```json
{
  "context": ["whole interaction history 50 messages"],
  "task": "Build summary"
}
```

**✅ Pattern OK:**
```json
{
  "prompt": "TASK: Build executive summary untuk topik X.\n\nKONTEKS: User butuh laporan untuk meeting Senin. Key points: A, B, C.\n\nCONSTRAINTS: Maks 200 kata, bahasa formal, no jargon.\n\nACCEPTANCE: 1) Exec summary di awal, 2) 3 bullet key insight, 3) Call-to-action di akhir.\n\nDELIVERABLE: ringkasan.md di /shared/<requester>/job/"
}
```

Implementor di section 35 wajib add validation di `internal/comms/delegate.go` — reject delegate request yang violate aturan.

### Marker di roadmap

Section yang berpotensi over-prompt sudah ditandai `⚠️ OVER-PROMPT RISK` di:

**Agent roadmap:** section 1 (episodic), 2 (mistakes), 11 (tools tier1), 27 (codemap engine)

**Router roadmap:** section 7 (mistakes global), 8 (skill catalog), 17 (knowledge share — plus context contamination risk)

Setiap implementor section di atas **WAJIB** baca standar ini dulu sebelum tulis kode injection ke prompt.

---

## 10. Collaboration & Governance (catatan strategis)

> Standar ini ditambahkan 2026-05-29 setelah review opsi voting/governance.

### Soft layer (ada / hari 1)

- **`peer_review` tool** — warga A bisa minta "second opinion" dari warga B
  atau dari router brain (lihat referensi
  `referensifile/section_11_tool_catalog_tier1/peer_review.go`).
  Soft governance — informational, tidak mengikat.
- **`approvals` shared library** — pattern owner approval workflow yang
  dipakai bersama oleh: scanner (section 25), zombie detector (section 29),
  protector custom rule (section 24). 1 library, jangan duplikasi
  (lihat `referensifile/_common/approvals/approvals.go`).
- **BFT 2-of-3 quorum** — sudah ada di mesh gossip (Router section 21)
  untuk emergency broadcast/revocation. Hard consensus untuk security event.

### Hard layer (defer P3 — formal voting governance)

Subsystem voting penuh (`vote_governance.go`, `vote_ballots`, `vote_casts`,
`votes` tables, `flowork-council` daemon) **belum di-roadmap**. Alasan:

- Single-owner sekarang (Mr.Dev). Owner decision cukup.
- Mr.Flow baru 1 warga aktif. Voting butuh ≥3 untuk meaningful quorum.
- Approval workflow scattered (scanner/zombie/protector) handle kasus owner
  decision. Pakai `_common/approvals/` library untuk konsistensi.
- BFT mesh handle security emergency tanpa butuh formal vote layer di atas.

### Trigger evaluasi (kapan re-consider hard layer)

Hard voting layer di-evaluate ulang kalau salah satu kondisi tercapai:

1. **Active warga ≥ 5** dengan tugas saling depend.
2. **Konflik antar-warga** yang ngga bisa di-resolve sama BFT mesh atau
   owner decision (mis. 2 warga claim resource sama).
3. **Constitution change** yang butuh formal vote multi-warga (selain
   owner approve).
4. **Successor / heir** ke owner — kalau Mr.Dev mati, governance harus
   ada untuk decide next-step (sesuai vision Flowork "survive without him").

Kalau trigger tercapai, port `flowork-council` daemon + `vote_governance.go`
dari `Music/flowork/brain/db/` jadi Bagian baru di roadmap.

---

## 12. Otak, Memori & Sistem Imun — Learning & Anti-Halu

> Ditambahkan 2026-06-04 setelah bangun sistem imun anti-halu (antibody injection
> + feedback + decay) di router. **Inti: agent DILARANG halu, dan harus makin pinter
> sendiri — TANPA retraining tiap saat.** WAJIB dibaca sebelum bikin agent yang
> mikir / dispatch tool / ngambil keputusan. Ini yang dulu kelewat → halu keulang.

### 12.0 Prinsip induk (hafalin)

**"deterministik = kuat, LLM lemah = rapuh."**

Logika kritis — routing, pilih kategori, gating keputusan — taruh di **KODE**, bukan
di LLM. LLM cuma buat yang beneran fuzzy (nulis, nyimpulin, ngobrol). **JANGAN PERNAH**
gantungin alur kritis ke kemampuan model manggil tool sendiri / inget aturannya sendiri
— model lemah (apalagi lokal 7B) bakal lupa/halu. **Paksa lewat kode atau injeksi.**

Contoh nyata: routing "analisa saham" → crew. Itu `deterministicRoute()` di
`agents/mr-flow/main.go` (kode), BUKAN ngarep LLM mutusin. Hasil: ga halu walau model lemah.

### 12.1 INGEST vs TRAINING — jangan ketuker

| | INGEST (RAG) | TRAINING (LoRA) |
|---|---|---|
| Knowledge ditaruh | Brain DB (SQLite, di luar model) | Bobot neural (di dalam model) |
| Update | **Instan** — add row, ready | **Mahal** — retrain GPU jam-jaman |
| Compute | CPU, cepat | GPU |
| Buat apa | Fakta, koreksi, knowledge | Reflex + gaya (BUKAN konten) |

- **DEFAULT: knowledge/fakta/koreksi → INGEST.** Murah, instan, updatable.
- **TRAINING cuma buat bake REFLEX/gaya**, bukan konten. Konten di-train = basi + mahal di-update.
- ⚠️ Reflex tool-dispatch lewat INGEST itu **halu-prone** (model mungkin ga nengok knowledge-nya)
  → makanya butuh **DETERMINISTIC INJECTION** (12.4), bukan ngarep model baca sendiri.

### 12.2 Karma — sinyal reward (kawin sama mistakes)

- `karma_self` (per-agent, counter / moving-average) + mesh karma (bounded 0–1 + **decay**).
- Karma = **bobot kepercayaan** sebuah pola. Pola kebukti bener → karma naik; pola salah →
  antibody-nya di-reinforce (12.4).
- Dipakai buat **nge-rank** apa yang di-surface ke model — yang karma tinggi diprioritasin.
- Tools: `karma_set` / `karma_query` (agent), `AdjustKarma`/`DecayKarma` (router mesh).

### 12.3 Edu-error & Mistakes journal — belajar dari salah

- Agent: `mistake_log` (catat salah) + `mistake_recall` (cek "pernah salah di konteks ini?"
  SEBELUM tindakan beresiko). `edu_error_upsert/lookup` buat error ber-kode.
- Promote ke router brain global: `POST /api/mistakes/submit` → `mistakes_journal`
  (tier=`global`, `hit_count` = reinforcement, kategori whitelist: logic/safety/performance/
  security/ux/governance).
- **ATURAN:** agent yang ngerjain hal beresiko WAJIB punya jalur recall — TAPI jangan ngarep
  model manggil sendiri (lihat 12.4).

### 12.4 Antibody Injection — sistem imun anti-halu (DETERMINISTIC, di GATEWAY)

Pipa yang bikin model lemah sekalipun **ga ngulang halu**. Hidup di **router (gateway)**,
bukan di agent — karena gateway liat SEMUA request + ga bisa di-skip model.

1. **INJECT** — mistakes relevan (skor = `karma(hit_count) × relevansi(overlap) × decay(recency)`,
   **MAX 3**) disuntik ke system prompt **SEBELUM** LLM. File: `flowork_Router/internal/router/mistakeenrich.go`.
2. **FEEDBACK** — response masih halu (mis. `task_run` kategori non-kanonik) → auto-`SubmitMistake`
   → karma antibody **NAIK** → next time lebih sering keinject. File: `mistakefeedback.go`. Self-learning.
3. **DECAY** — antibody yang berhenti relevan **pudar** otomatis (half-life 30 hari, floor 0.1).
4. **BUKTI**: kategori halu **0/3 → 3/3** bener setelah inject; feedback live (`olahraga` → karma +3).

**Kenapa di gateway, bukan di agent:** model 7B flaky **ga bakal inisiatif** manggil `mistake_recall`.
Gateway **maksa**. Prinsip: **enforcement > imbauan.**

### 12.5 Wiring Invariant Guard — jaga pipa kritis dari "AI suka rubah jalur"

- Pipa kritis (antibody hook, deterministic route, dll) didaftar di
  `Flowork_Agent/internal/scanner/auditors_invariant.go`. Putus / pola hilang → **CRITICAL**
  di Threat Radar **otomatis** (scanner auto-jalan tiap file berubah + startup).
- **ATURAN BUAT AI MANAPUN (TERMASUK PASCA-COMPACT):** JANGAN ubah / pindah / cabut jalur
  kritis tanpa izin **eksplisit** Mr.Dev. **Nambah invariant = boleh. Ngurangin = DILARANG.**
- Kenapa: akar masalah lama = "AI suka rubah jalur" → arsitektur bagus keputus → halu balik.
  Lock-comment itu pasif (bisa diabaikan); guard ini **aktif** (kode yang teriak).

### 12.6 Pemilihan model per agent (learning 2026-06-04 — JANGAN keulang)

Test live semua agent di Qwen-7B-abliterate lokal:
- **mr-flow (komandan/percakapan) → ANCUR** di qwen — nyasar ke bahasa lain, halu berat (ceiling 7B Q4 + base Mandarin bocor).
- **Crew atomik 1-tugas simpel (mis. music-riset) → lumayan** di qwen.

**ATURAN:**
- Agent **komandan / reasoning / percakapan** → model KUAT (Claude). Jangan dipaksain ke lokal.
- Agent **atomik 1-tugas simpel** → boleh lokal (qwen — sovereign, no kuota, uncensor).
- **Hybrid.** Set per agent via `kv.router_model`; router yang map nama→provider (model policy di ROUTER, bukan hardcode di agent).
- Antibody injection berlaku ke SEMUA model (di gateway) — bikin lokal lebih aman, **tapi ga
  nyembuhin ceiling 7B** buat tugas berat. Pilih model sesuai beban tugas.

---

## 13. Checklist anti-halu & learning (gabung ke section 8 sebelum merge)

Selain checklist isolation (section 8), agent yang **mikir / dispatch / ngambil keputusan**
WAJIB centang:

- [ ] Logika kritis **deterministik di kode**, bukan ngandelin LLM manggil tool? (12.0)
- [ ] Knowledge/koreksi baru → **INGEST ke brain**, bukan hardcode / over-prompt / di-train? (12.1)
- [ ] Tugas beresiko punya jalur **mistake_recall** + ke-cover **antibody injection** di gateway? (12.3–12.4)
- [ ] Halu yang ke-detect **nutup loop** (feedback → karma)? (12.4)
- [ ] **Model dipilih sesuai beban** — komandan/reasoning = kuat, atomik simpel = boleh lokal? (12.6)
- [ ] Ga ada **pipa kritis** yang diubah/dipindah tanpa izin? (12.5)
- [ ] Prompt budget ≤ 30% context (section 11) — antibody MAX 3, mistakes ga growing?

---

*Living document. Update tiap kali konsep berubah atau ada keputusan
arsitektur baru. Tanggal terakhir: 2026-06-04 (tambah Section 12-13: learning/imun/anti-halu).*
