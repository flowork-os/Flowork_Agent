# GROUP — Arsitektur Squad/Koloni Flowork: Cara Kerja, Filtur & Cara Bikin

> Dokumen referensi (white-label). Menjelaskan SEMUA soal GROUP/SQUAD: cara kerja,
> filtur, cara bikin, file penghubung, titik-extension (cabang), dan daftar file FREEZE.
> Owner: Aola Sahidin (Mr.Dev). Repo: https://github.com/flowork-os/Flowork-OS.
> Update terakhir: 2026-06-23.
> ⚠️ File ini KE-TRACK repo → NOL data personal owner (mekanisme generic doang).

---

## ⛔ WAJIB BACA DULU (buat AI/dev yang mau ngedit)

File-file inti GROUP di bawah ini **DI-FREEZE** (chattr +i + hash di `KERNEL_FREEZE.md`).
**JANGAN buka / edit file frozen.** Kalau lo mau nambah filtur group:

1. **Bikin group/squad baru** → TIDAK perlu ngoding. Itu DATA (lihat §4 "Cara Bikin Group").
2. **Ubah perilaku worker/synth** → pakai field `WorkerDirective`/`SynthDirective` per-kategori (DATA, §3).
3. **Mode eksekusi baru** (parallel/debate/vote) → daftar di **`taskflow_ext.go`** (NON-frozen) lewat `RegisterExecStrategy` (§6).
4. **Target sync baru / saklar slash** → **`groupsapi_ext.go`** (NON-frozen) lewat `RegisterGroupSyncHook` / env switch (§6).

Filosofi owner: **"file frozen ngak akan pernah dibuka lagi"** — semua filtur masa depan masuk
ke file CABANG `*_ext.go`. Kalau lo (AI berikutnya) ngerasa HARUS edit file frozen → STOP,
hampir pasti ada jalur cabang/data yang bener. Minta izin owner kalau bener-bener mentok.

---

## 0. FILOSOFI INTI — "Koloni Semut"

GROUP = **1 tugas dipecah ke banyak agent kecil yang fokus, hasilnya digabung jadi 1 jawaban.**
Prinsip "agent bodoh, engine pinter": tiap anggota cuma jago 1 hal (anti over-prompt), engine
yang nyusun alurnya. Alur kanonik:

```
MR.FLOW (orchestrator/router)
   │  sadar tugas X → cocok ke squad Y
   ▼
SQUAD/CATEGORY Y  (crew + synthesizer)
   │  fan-out ke tiap anggota (fokus) → tiap anggota tulis hasil ke FILE
   ▼
SYNTHESIZER  baca semua file → 1 keputusan/jawaban final (fan-in)
```

Anti-halu: prompt tiap worker KECIL + data antar-agent lewat **FILE** (`/shared/tasks/...`),
BUKAN prompt-chaining (yang numpuk konteks → halu).

---

## 1. DUA SUBSISTEM "GROUP" (penting — jangan ketuker)

Flowork punya **dua** representasi group yang paralel. Tahu bedanya = ga bingung.

| | **A. TASKFLOW CATEGORY** (AKTIF) | **B. GROUPSAPI MODULE** (slash, di-deprecate) |
|---|---|---|
| Apa | Definisi crew+synth di DB (`floworkdb`) | Modul loket `<id>.fwagent` ber-kv `group=1` |
| Eksekutor | `taskflow.RunCategoryTask` (sequential→synth) | group-template WASM (mode parallel/debate) |
| Dipanggil | `task_run(category=...)` oleh mr-flow | slash Telegram `/namagroup` |
| Awareness | `task_list` (mr-flow baca → route) ✅ | `mr-flow-next` baca kv `groups` |
| Status | **JALUR UTAMA** — andalan kesadaran mr-flow | **MATI** (mr-flow-next ga pernah ke-deploy) |

**Keputusan owner 2026-06-23:** andelin **KESADARAN mr-flow** (jalur A). Slash (jalur B) **dibuang**
(setMyCommands dikosongin + di-gate saklar, lihat §6). Jadi yang BENER-BENER routing tugas =
**TASKFLOW CATEGORY**. GroupsAPI tetap dipakai buat GUI "Groups" tab (kelola modul group) +
tetap nulis roster, tapi push slash-nya MATI default.

> Catatan migrasi: `mr-flow-next` = inkarnasi "loket-native" orchestrator (Phase B) yang
> BELUM di-deploy (folder cuma punya `workspace/`, ga ada wasm/manifest → ga pernah ke-load).
> Selama dia mati, jalur slash (B) ga fungsi. Kalau migrasi kelar nanti → idupin slash via
> env `FLOWORK_GROUP_SLASH=1` (TANPA unfreeze, §6).

---

## 2. CARA KERJA — TASKFLOW CATEGORY (jalur aktif, end-to-end)

File inti: `internal/taskflow/taskflow.go` (core) + `internal/taskflow/taskflow_retask.go`
(worker/synth/self-heal) + `taskflow_handler.go` (HTTP + storage) + `internal/tools/builtins/taskflow_tools.go`
(tool `task_list`/`task_run`).

Alur `RunCategoryTask(ctx, host, sharedDir, cat, input, runID, rec)`:

1. **Validasi**: crew kosong / synth kosong / input kosong → error cepat.
2. **CABANG hook** (`extRunCategory`, NON-frozen): kalau ada `ExecStrategy` terdaftar yang KLAIM
   kategori → dia ambil-alih (mode parallel/debate/dst). nil → lanjut default sequential. (§6)
3. **Fan-out**: tiap anggota crew di-invoke **berurutan** (`invokeWorker`). Tiap worker nulis
   output ke file `<shared>/<agent>/job/run-<runID>__<agent>.md`. Engine COPY file tiap worker
   ke dir job synthesizer (karena shared workspace itu PER-AGENT, ga otomatis kebaca lintas-agent).
4. **Fan-in**: `invokeSynth` — synthesizer baca semua file → 1 keputusan.
5. **SELF-HEAL (RETASK)**: kalau synth deteksi data 1 worker SALAH → output `RETASK <peran>: <koreksi>`
   → engine kasih tugas ulang ke worker itu → synth ulang. Max `maxRetaskRounds` (anti infinite).
6. Hasil: `Result{Recommendation, Steps[], RunID, ...}` — di-persist ke DB (timeline) kalau ada Recorder.

**Tool yang dipakai mr-flow:**
- `task_list` — daftar category yang ada (id, name, trigger_hint). mr-flow PANGGIL ini buat tau
  squad apa aja sebelum route.
- `task_run(category, subject)` — trigger 1 category. ASYNC → balik `run_id`, hasil di belakang
  (~beberapa menit). Param `group` (opsional, INTERNAL) = delegasi ke GroupsAPI module (jarang).

---

## 3. DATA MODEL — `Category` (sumber kebenaran squad)

`taskflow.Category` (di-load caller dari `floworkdb`, BUKAN hardcode):

| Field | Isi |
|---|---|
| `ID` | slug unik (mis. `facebook`, `stock-analyst-squad`) |
| `Name` | nama tampilan (mis. "Facebook Content") |
| `Crew []CrewMember` | anggota fan-out — tiap `{AgentID, RoleLabel}`, urut = urutan eksekusi |
| `Synthesizer` | agent id yang fan-in → keputusan final |
| `SynthDirective` | OPSIONAL — override format keputusan synth. Kosong = default finansial (BUY/HOLD/AVOID) |
| `WorkerDirective` | OPSIONAL — cara worker kerja. Kosong = "cari data REAL, jangan ngarang". Diisi buat kategori KREATIF (mis. zodiak: ngarang ramalan = MEMANG tugasnya) |

**KUNCI plug-and-play:** mau perilaku squad beda (format output, gaya kerja) → cukup isi
`SynthDirective`/`WorkerDirective` (DATA di DB) — **TANPA ngoding**. Storage: `floworkdb`
(`UpsertCategory`); diakses HTTP `/api/taskflow/category*`.

Contoh nyata (squad Facebook):
- Crew seq: `fb-repofinder` (cari 1 repo GitHub nyata) → `fb-writer` (tulis status Inggris pros/cons).
- Synth: `fbspecial` (poster, model Opus).
- trigger_hint: "posting/upload konten ke Facebook…" → mr-flow cocokin pas owner bilang "upload ke fb".

---

## 4. CARA BIKIN GROUP/SQUAD BARU (3 jalan — semua TANPA ngedit file frozen)

### 4.1 Lewat AI Studio Architect (paling gampang) — `architect.go`
`POST /api/architect/build {prompt, model}` → 1 panggilan Opus desain spesialis + lead →
`installPluginPack` (bikin agent anggota) → `UpsertCategory` (daftar category+crew+synth+directive)
→ `groups.CreateGroup` (modul koordinator). Sekali jalan: squad + awareness mr-flow langsung nyala.
Ini yang dipakai bikin saham/crypto/primbon.

### 4.2 Manual per-item (kontrol penuh) — pola "koloni semut"
1. Bikin tiap agent anggota dari `agent-template` (lihat `scripts/mk-agent.sh`): 1 agent = 1 tugas,
   persona fokus, model per-agent (`router_model` kv).
2. Daftar Category: `UpsertCategory` (atau HTTP) — isi `Crew[]` (urut), `Synthesizer`, `trigger_hint`,
   directive kalau perlu.
3. (Opsional) Bikin GroupsAPI module: `POST /api/groups/create {id, display_name}` + `/api/groups/config`.

### 4.3 Lewat GUI "Groups" tab — `web/tabs/groups.js`
Tombol **+ Buat Group** → create → drag anggota + pilih synthesizer + set task/mode → Save (config).
Cocok buat ngerakit dari agent yang udah ada.

**Catatan:** biar mr-flow SADAR squad baru, yang wajib = **Category** ke-daftar (jalur A). GroupsAPI
module (jalur B) opsional sekarang (slash mati).

---

## 5. ROUTING — gimana mr-flow SADAR + delegasi (anti-nyasar)

Ini yang bikin "ketik 'upload ke facebook' → mr-flow panggil squad". **2 syarat WAJIB**, dua-duanya
udah dibenerin 2026-06-23:

1. **Tool router ke-EXPOSE** — `task_list` + `task_run` HARUS di `primaryExtraTools`
   (`internal/agentmgr/tool_specs.go`), BUKAN cuma subscription. Alasan: mr-flow ~196 subs > cap →
   tool subscription KE-DROP → kalau task_list ke-drop, mr-flow ga bisa route → nyasar ke
   brain_search_shared. `maxExposedTools` dinaikin biar muat (cap 55).
2. **Persona nyuruh route** — persona mr-flow (kv `prompt`) punya blok **"ROUTER TEAM"**: *"SELALU
   baca task_list, cocokin ke trigger_hint, task_run; JANGAN hardcode id kategori"*. DINAMIS →
   category baru auto-routable tanpa ubah persona (plug-and-play).

> Edit persona mr-flow AMAN: `store.Save()` (POST `/api/agents/config`) itu **FULL-REPLACE** —
> field absen = DIHAPUS. WAJIB GET config utuh dulu, ubah `prompt` aja, POST balik UTUH (tools+skills+
> secrets). Secrets ke-mask di GET → di-reconcile pas POST.

Bukti live: probe "upload konten ke facebook" → mr-flow jawab "nyalain crew facebook… lewat crew".

---

## 6. CABANG ABADI (extension point) — biar file frozen GA PERNAH dibuka lagi

Realisasi perintah owner: *"kasih cabang file/switch biar file frozen ngak akan pernah dibuka lagi"*.
Tiap file frozen punya pasangan `*_ext.go` NON-frozen. File frozen cuma MANGGIL fungsi di ext.

### 6.1 `internal/groupsapi/groupsapi_ext.go` (NON-frozen)
- `slashPushEnabled() bool` — saklar push slash Telegram. **DEFAULT MATI** (awareness-only).
  `orchestrator.go` (frozen) manggil ini; pas mati → push KOSONG (menu Telegram tetep bersih).
  **Idupin slash lagi: env `FLOWORK_GROUP_SLASH=1`** — tanpa unfreeze.
- `RegisterGroupSyncHook(fn)` — target sync masa depan (menu Discord/WhatsApp, registry, metrik)
  daftar di sini. `SyncToOrchestrator` (frozen) manggil `runGroupSyncHooks(parts)` tiap sync.

### 6.2 `internal/taskflow/taskflow_ext.go` (NON-frozen)
- `RegisterExecStrategy(fn)` — mode eksekusi crew baru (parallel/debate/vote/dll). `RunCategoryTask`
  (frozen) manggil `extRunCategory(...)` di awal: strategi yang KLAIM kategori (balik non-nil) ambil
  alih; nil → default sequential (yang udah kebukti gate saham). Strategi panic ≠ ngerusak (recover).

### 6.3 Yang udah plug-and-play (ga butuh hook, murni DATA)
- **Group/squad baru** = data (Category + agent anggota). Ga ada code per-group.
- **Perilaku worker/synth** = `WorkerDirective`/`SynthDirective` (data per-kategori).
- **Roster on/off, rename, anggota** = kv di loket store (GUI Groups tab).

---

## 7. FILTUR GROUP (daftar lengkap — GUI + API)

### GroupsAPI (`/api/groups/*`, file `groupsapi.go` + handler) — tab "Groups"
| Filtur | Endpoint | Catatan |
|---|---|---|
| List group + pool anggota | `GET /api/groups` | group = modul kv group=1; pool = agent eligible |
| Bikin group | `POST /api/groups/create {id,display_name}` | deploy modul koordinator dari group-wasm |
| Set roster | `POST /api/groups/config?id= {members,synthesizer,task,mode,debate_rounds}` | tulis ke loket, mirror ke group.json, re-sync |
| Toggle on/off | `POST /api/groups/toggle?id=&disabled=` | cascade ke SEMUA anggota (off = unload member) |
| Hapus group | `POST /api/groups/delete?id=` | disable anggota DULU (gateway), baru hapus folder |
| Reset | `POST /api/groups/reset` | restore dari seed repo (group.json) |
| Mode | kv `mode` = `parallel`\|`debate` (+`debate_rounds`) | buat group-wasm (jalur B) |

### Taskflow (`/api/taskflow/*`) — category routing (jalur aktif)
| Filtur | Endpoint |
|---|---|
| Daftar category | `GET /api/taskflow/categories` |
| Detail category | `GET /api/taskflow/category?id=` |
| Hapus category | `POST /api/taskflow/category/delete` |
| Trigger run | `POST /api/taskflow/run` (atau tool `task_run`) |
| History run | `GET /api/taskflow/runs?category=` |
| Detail run | `GET /api/taskflow/run-detail` |
| Schedule (jadwal otomatis) | `GET /api/taskflow/schedules`, `POST /api/taskflow/schedule`, `/schedule/delete` |

### Guard penting (jangan diapus)
- `eligibleMember`: channel/operator-*/mr-flow*/scanner/service **DILARANG** jadi anggota
  (operator-* punya cap power-off → fan-out bisa matiin mesin). Hard-exclude.
- `DeleteHandler`/`ToggleHandler` OFF: cascade disable anggota (group = pintu masuk; anggota
  ga boleh tetep ke-invoke pas group mati/dihapus).
- `idRe` `^[a-z0-9][a-z0-9-]{1,39}$`: id = nama folder + modul → cegah path-escape.

---

## 8. PETA FILE GROUP (file → peran → status freeze)

| File | Peran | Freeze |
|---|---|---|
| `internal/taskflow/taskflow.go` | CORE executor (fan-out→synth+retask) | **FREEZE** |
| `internal/taskflow/taskflow_retask.go` | invokeWorker/invokeSynth/parseRetask | **FREEZE** |
| `internal/taskflow/taskflow_ext.go` | CABANG: RegisterExecStrategy | NON-frozen |
| `internal/tools/builtins/taskflow_tools.go` | tool `task_list`/`task_run` | **FREEZE** |
| `internal/groupsapi/groupsapi.go` | CRUD group (list/config/create/delete) | **FREEZE** |
| `internal/groupsapi/orchestrator.go` | SyncToOrchestrator (roster + slash gate) | **FREEZE** |
| `internal/groupsapi/create_team.go` | CreateGroup (buat Architect) | **FREEZE** |
| `internal/groupsapi/toggle.go` | ToggleHandler (cascade on/off) | **FREEZE** |
| `internal/groupsapi/seed.go` | writeGroupSeed / SeedFromJSON (group.json) | **FREEZE** |
| `internal/groupsapi/reset.go` | ResetHandler (restore dari repo) | **FREEZE** |
| `internal/groupsapi/telegram_commands.go` | syncTelegramCommands (setMyCommands) | **FREEZE** |
| `internal/groupsapi/groupsapi_ext.go` | CABANG: slash switch + sync hook | NON-frozen |

Catatan: `tool_specs.go` (expose task_list/task_run) + persona mr-flow (kv) = **enabler routing**,
TAPI bukan file group-core → tetap header-lock (BUKAN hash-frozen), karena tool-exposure & persona
sesekali di-tune. Jangan freeze itu bareng group.

---

## 9. RINGKAS — "siapa nyambung ke siapa"

```
owner ──"upload ke fb"──▶ mr-flow (persona ROUTER + tool task_list/task_run [tool_specs.go])
                              │ task_list → liat Category
                              │ task_run(category=facebook, subject=...)
                              ▼
                    taskflow.RunCategoryTask  [taskflow.go FROZEN]
                       ├─ extRunCategory()? ──▶ [taskflow_ext.go] strategi mode baru (nil=default)
                       ├─ fan-out crew (seq) ──▶ fb-repofinder → fb-writer  (tulis file)
                       └─ fan-in synth ───────▶ fbspecial  → keputusan/post
GroupsAPI [groupsapi.go FROZEN] ── kelola modul group (GUI) ── SyncToOrchestrator
                       └─ slashPushEnabled()? [groupsapi_ext.go] ──▶ default MATI (awareness-only)
```

---

## 10. PANTANGAN (jangan diulang)

- ❌ Jangan hardcode daftar kategori di persona/kode ("saham/crypto/...") — bikin DINAMIS via task_list.
  (Akar bug 2026-06-23: persona lama hardcode → facebook ditolak → nyasar.)
- ❌ Jangan andelin slash GroupsAPI buat routing — orchestrator (mr-flow-next) belum deploy. Andelin
  KESADARAN mr-flow (Category).
- ❌ Jangan masukin operator-*/channel jadi anggota group (bisa matiin mesin / I/O bukan analis).
- ❌ Jangan POST `/api/groups/delete` kalau cuma mau bersihin slash — itu nge-DISABLE anggota. Hapus
  folder manual atau pakai mekanisme yang bener.
- ❌ Jangan buka file FROZEN buat filtur baru — pakai `*_ext.go` (§6) atau DATA (§4).
