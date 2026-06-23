# TOOLS — Arsitektur Tool Flowork: Cara Kerja, Cara Bikin, Kenapa SIDECAR, Self-Evolving & FREEZE

> Dokumen referensi KANONIK (white-label). Menjelaskan SEMUA soal TOOLS: cara kerja, cara bikin tool
> (manual & agent-bikin-sendiri), kenapa harus SIDECAR, lifecycle self-evolving, guardrail, titik-extension
> (cabang/switch), dan daftar file FREEZE.
> Owner: Aola Sahidin (Mr.Dev). Repo: https://github.com/flowork-os/Flowork-OS · floworkos.com
> Update terakhir: 2026-06-23.
> ⚠️ File ini KE-TRACK repo → NOL data personal owner (mekanisme generic doang).
> 🔒 6 file `tools-core` di-FREEZE (chattr +i + hash di KERNEL_FREEZE.md) merujuk dok INI. Kalau lo AI yang
>    mau ngedit file ber-header `🔒 FROZEN tools-core` → BACA dok ini DULU. 99% perluasan TIDAK perlu buka
>    file frozen — ada CABANG/SWITCH (§7). Buka frozen = keputusan SADAR + izin owner (§9).

---

## 1. RINGKAS — apa itu "tool" di Flowork

Tool = kapabilitas yang bisa dipanggil agent (LLM) lewat tool-call. Ada 2 kelas:

1. **Builtin** — di-compile masuk ke binary (`internal/tools/builtins/*.go`), register via `init()`. Cepet,
   inti, ga bisa di-upload. Contoh: `file_read`, `web_search`, `telegram_send`, `tool_create`, `tool_search`.
2. **SIDECAR** (bintang dok ini) — tool PLUG-AND-PLAY: tiap tool = **folder self-contained** `tools/<name>/`
   yang di-compile jadi **binary native sendiri** lalu di-**exec host sbg proses terpisah**. Inilah yang bikin
   tool "beneran kerja" (privileged: shell/fs/browser/native-lib) jadi **POSTABLE** kaya plugin WordPress.

Tool sidecar ada 2 asal: **bawaan** (di-drop manual di repo) & **agent-bikin-sendiri** (`tool_create`,
self-evolving — §5).

---

## 2. KENAPA SIDECAR? (keputusan arsitektur inti)

Owner pengen tool yang "truly postable" — bisa ditambah tanpa rebuild seluruh kernel, tanpa kena sandbox.
3 pilihan dipertimbangin:

| Pendekatan        | Privileged? | Postable? | Isolasi? | Verdict |
|-------------------|-------------|-----------|----------|---------|
| Compiled-in builtin | ya        | ❌ (rebuild kernel frozen) | ❌ (nyatu proses) | cuma buat inti |
| WASM `.fwpack`    | ❌ (sandbox) | ✅        | ✅        | ga bisa shell/fs/browser |
| **SIDECAR (native, exec)** | ✅ | ✅ | ✅ (proses terpisah) | **DIPILIH** |

**Sidecar menang** karena native (lepas sandbox WASM → bisa privileged) + modular (drop folder, ga rebuild
kernel) + isolasi proses (tool crash/jahat ga nyentuh kernel). Konteks: ~125 dari 183 tool privileged GA bisa
jadi `.fwpack` (kena sandbox) → sidecar nutup gap itu. Detail: `docs/ROADMAP_MULTI_OS_TOOLS.md` §14.

---

## 3. PRINSIP ISOLASI — yang bikin plug-and-play + agnostic (SYARAT KERAS OWNER)

> "tools itu terisolasi — misal tools butuh library, library-nya ADA DI DALAM FOLDER DIA SENDIRI, ngak boleh
> ada library SHARE. Ini yang bikin plug-and-play dan agnostic." — owner 2026-06-23

Tiap tool = folder `tools/<name>/` dengan **`go.mod` SENDIRI** + dependency-nya **vendored di folder itu**.
**NOL shared library** antar-tool. Akibatnya:
- Drop folder → ke-discover → build → jalan. Cabut folder → tool ilang. Murni plug-and-play.
- Tool A rusak/ganti dep ga ngefek ke tool B (agnostic, ga ada dependency-hell bareng).
- Tool bisa di-share antar-instance Flowork (kirim folder doang, self-contained).

---

## 4. ABI — kontrak abadi (JANGAN diubah)

Host meng-EXEC binary tool, komunikasi via stdin/stdout JSON, stateless per-panggil:

```
STDIN  (host→tool):  {"args": { ... }}
STDOUT (tool→host):  {"output": <any>, "error": "<string kosong kalau sukses>"}
exit 0
```

- Proses FRESH tiap panggil → ga ada state bocor, ga ada port bentrok.
- CWD tool = folder-nya sendiri → bisa baca aset relatif (self-contained).
- `error` non-kosong → host anggap gagal (+ catat buat GC, §6).
- Timeout 90 detik/panggil (di `sidecarTool.Run`).

Struktur folder minimal:
```
tools/<name>/
  go.mod          # modul sendiri (isolasi)
  main.go         # baca stdin → kerja → tulis stdout (ABI di atas)
  tool.json       # manifest: name, capability, description, params[], returns
  <name>          # binary hasil build (di-exec host)
```

`tool.json` → **`capability: ""`** = NO gating (semua agent bisa pake, kaya `echo`). Tool PRIVILEGED isi
cap-nya sendiri (mis. `"exec:shell"`) → otomatis ke-gate broker (cuma agent ber-cap yg boleh).

---

## 5. CARA BIKIN TOOL

### 5a. Manual (developer drop folder)
1. Bikin `tools/<name>/` (main.go ABI + tool.json + go.mod), lib di folder itu.
2. Build: `tools/build-tools.sh` (per-modul `GOWORK=off`).
3. Reload: restart host ATAU `POST /api/tools/sidecar` (re-discover). Cek `tools/README.md`.

### 5b. Agent bikin SENDIRI — `tool_create` (SELF-EVOLVING, inti visi owner)
> "SEMUA agent bisa bikin tools di `/tools` — itu PALING penting. Flowork gw desain buat BEREVOLUSI, dan
>  inilah tempat paling sempurna buat tumbuh." — owner 2026-06-23

Agent panggil builtin `tool_create{name, description, params, code}`. Host:
1. Validasi nama (`^[a-z][a-z0-9_]{1,39}$`, unik global anti-nimpa-builtin).
2. **Anti-eskalasi**: tolak import bahaya (denylist di `toolsidecar_ext.go` — §7). Fase 1, sebelum sandbox-OS.
3. Scaffold `tools/_private/<agent_id>/<name>/` (go.mod sendiri) + wrap boilerplate ABI + tulis `code` agent.
4. **Build-verify**: gagal compile → `build_log` balik ke agent → agent benerin → retry = **LOOP BELAJAR**.
5. Sukses → register **PRIVAT**: cuma si pembuat yang liat (specs) + pake (run). Agent lain ga liat sama sekali.

Agent tau cara ini lewat **error-edukasi** (§6 deletion + FASE 3): pas tool ga ketemu, host kasih petunjuk
`tool_search` dulu → kalau bener ga ada → ajarin `tool_create`. Jadi agent ga pernah buntu — diajarin tumbuh.

---

## 6. LIFECYCLE SELF-EVOLVING (lahir → dewasa → mati → sadar)

```
tool_create (PRIVAT) ──> dipake (track use/error) ──> Dewan review (promote-tool) ──> SHARED (semua agent)
                                                                                          │
                          ┌───────────────────────────────────────────────────────────────┘
                          ▼
            GC: error-tinggi / nganggur-lama ──> MATI (unregister + hapus folder + tombstone)
                          │
                          ▼
            DELETION-AWARE: agent SADAR tool mati (sampai ke OTAK) ──> ga halu tool-hantu
```

### Promote PRIVAT → SHARED (NO acc owner — biar hidup walau owner ga ada)
Tool privat di-antri jadi **EvolveProposal kind `promote-tool`** → di-review **Dewan self-evolution** (tim agent
adversarial: Pembela ⚔️ Penantang → Hakim panel-3; CONFIGURABLE model di GUI, **BUKAN hardcode**, **BUKAN acc
owner**). Lolos → pindah `tools/_private/<agent>/<name>/` → `tools/<name>/`, register shared, ke-expose semua
agent. Reuse Dewan yang udah ada (ga bikin tim baru).

### AUTO-GC — seleksi alam
Tiap tool track `error_count` + `last_used` (`tools/.health.json`). Cron tiap 6 jam + `POST /api/tools/gc`:
- **error ≥ N** (default 5) → tool rusak (mis. API berubah/mati) → HAPUS.
- **nganggur > N hari** (default 90) → obsolete/sementara → HAPUS.
Tool bawaan (agentID `""`) di-SKIP (ga ke-GC). Switch ambang = ENV (§7).

### DELETION-AWARE — agent sadar tool mati, sampai OTAK (KRUSIAL, "matang")
Hapus dari registry doang = DANGKAL — otak agent bisa udah "inget" tool itu (masuk lewat **dream** jadi
node/instinct) → agent halu nyoba tool HANTU. Jadi pas tool mati, 2 lapis:
1. **PRIMER**: unregister → ilang dari specs → pas agent coba akses → reactive `ERR_TOOL_GC_REMOVED`
   ("DULU ada, udah dihapus seleksi-alam, jangan akses bangkainya, bikin baru kalau perlu").
2. **MATANG (cognition)**: `tombstoneSweep` tiap GC → quarantine cognitive-node `agent:<id>/tool/<nama>`
   (excluded dari recall) + turunin confidence instinct yg nyebut tool mati (×0.3, floor 0.05 → konvergen).
   Tombstone-based = re-quarantine tiap sweep → nutup celah dream re-project tool-hantu dari pengalaman lama.

---

## 7. ⭐ CABANG / SWITCH — cara NAMBAH filtur TANPA buka file frozen

> Aturan owner: sebelum freeze, pikirin kemungkinan filtur baru → kasih cabang/switch biar file frozen
> GA PERNAH dibuka lagi. Ini daftar jalan-pintasnya. **Mulai dari sini SEBELUM mikir unfreeze.**

| Mau ngapain | EDIT DI SINI (non-frozen) | Jangan sentuh |
|-------------|---------------------------|---------------|
| Ubah kebijakan import bahaya (izinin/blok import baru) | `internal/toolsidecar/toolsidecar_ext.go` (`dangerImports`) | toolsidecar.go |
| Ubah/ tambah pelajaran error (mis. ERR_TOOL_*) | `internal/agentdb/edu_errors_ext.go` (`ExtraEduErrors`, DO-UPDATE override) | edu_errors_seed.go (frozen), tool_notfound_edu.go |
| Atur ambang GC (error/idle) atau matiin GC | ENV: `FLOWORK_TOOL_GC_MAXERR`, `FLOWORK_TOOL_GC_IDLE_DAYS`, `FLOWORK_TOOL_GC_OFF` | feature_tools_gc.go |
| Pindah lokasi folder tools | ENV: `FLOWORK_TOOLS_DIR` | toolsidecar.go |
| Ganti model/anggota tim review promote | Dewan self-evolution group di GUI (configurable) | feature_tools_promote.go |
| Expose tool baru ke semua/primary agent | `internal/agentmgr/tool_specs.go` (`coreExposedTools` / `primaryExtraTools` — sengaja NON-frozen, daftar tumbuh) | — |
| Tambah KAPABILITAS tools yang lebih besar (mis. tipe param baru, privileged-create flow, scope "team") | **bikin file `feature_*.go` BARU** (`init()`→`RegisterFeature`) — pola plug-and-play, main.go frozen ga disentuh | file feature lama |
| Tambah tool baru (manual / agent) | folder `tools/<name>/` atau `tool_create` | — |

Kalau kebutuhan lo ada di tabel → kerjain di kolom tengah, SELESAI, ga usah unfreeze.

---

## 8. FILE MAP — frozen vs non-frozen

### 🔒 FROZEN tools-core (6 file — chattr +i + hash KERNEL_FREEZE.md, header nunjuk dok ini)
- `internal/toolsidecar/toolsidecar.go` — engine: ABI exec, Discover/Register, CreateTool (scaffold+build-verify),
  Promote, DeleteTool, GCScan, health, Tombstones, ToolsDir.
- `internal/tools/builtins/tool_create.go` — builtin entry `tool_create` (glue tipis ke engine).
- `feature_tools_sidecar.go` — wiring discover + endpoint `GET/POST /api/tools/sidecar`.
- `feature_tools_promote.go` — pipeline privat→Dewan→shared (`autoProposePrivateTools`, `promoteToolApply`).
- `feature_tools_gc.go` — GC + deletion-aware (`runToolGC`, `tombstoneSweep`).
- `internal/agentmgr/tool_notfound_edu.go` — FASE 3: rekomendasi sepadan + ajakan `tool_create` + sinyal GC.

### 🔒 Sudah frozen sebelumnya (boundary dispatch + keamanan)
`internal/tools/registry.go` · `sandbox.go` · `sandbox_v3.go` · `interceptors.go` — registry inti + sandbox
capability-gate. (Lihat KERNEL_FREEZE.md "SHA256 manifest".)

### ✏️ NON-frozen by-design (CABANG/SWITCH + evolutif) — JANGAN di-freeze
- `internal/toolsidecar/toolsidecar_ext.go` — CABANG kebijakan import.
- `internal/agentdb/edu_errors_ext.go` — CABANG konten edu-error (override DO-UPDATE; di-seed `provision_dna.go`).
- `internal/agentmgr/tool_specs.go` — daftar expose tool (tumbuh; agentmgr by-doctrine non-frozen).
- `internal/agentmgr/tool_subscriptions.go` — `localSuggest` + subscription (subsistem lebih luas).
- `selfevolve_apply.go` — applier switch (kind evolve nambah; soft-lock).
- `tools/*` — folder tool itu sendiri (data evolutif). `tools/build-tools.sh`, `tools/README.md`.

---

## 9. UNFREEZE (kalau BENER-BENER perlu — keputusan sadar + izin owner)

Coba §7 dulu. Kalau perubahan emang harus di file frozen:
```bash
# 1. unfreeze OS-layer
sudo chattr -i <file>
# 2. edit (hati-hati — ini ABI/mekanisme inti)
# 3. gofmt + build + vet + test
gofmt -w <file>; (cd agent && GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=mod go build ./... && go vet ./...)
# 4. regenerate hash di KERNEL_FREEZE.md (symlink → flowork-secrets/KERNEL_FREEZE.md)
sha256sum <file>   # ganti baris lama
# 5. TestKernelFreeze harus ijo
(cd agent && go test ./ -run TestKernelFreeze -count=1)
# 6. re-freeze OS-layer
sudo chattr +i <file>
```
Catat alasan unfreeze di KERNEL_FREEZE.md (pola entri bertanggal yang udah ada).

---

## 10. CARA TES (bukti hidup)
- Sidecar discover: `POST /api/tools/sidecar` → daftar tool + scope.
- Agent bikin tool: `POST /api/agents/tools/run?id=<agent>` body `tool_create{...}` → `{ok, scope:private, build_log}`.
- Anti-eskalasi: `tool_create` dengan `os/exec` → ditolak.
- GC: bikin tool error 5× → `POST /api/tools/gc` → ke-prune (folder GONE + tombstone).
- Deletion-aware: panggil tool yg udah di-GC → pesan `ERR_TOOL_GC_REMOVED`.
- Freeze: `go test ./ -run TestKernelFreeze` → ijo (150 file inti).

Blueprint penuh: `docs/ROADMAP_MULTI_OS_TOOLS.md` §14 (sidecar) + §15 (self-evolving, sampai §15.8 status).
