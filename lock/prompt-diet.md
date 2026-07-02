# PROMPT-DIET — enrichment selektif + budget agregat + sticky-union tools (router)

> Owner: Aola Sahidin (Mr.Dev) · 2026-07-02. Roadmap F-A1/F-A3 + akar "sering kena limit".
> Seam dipasang atas mandat owner ("file lock yang ngak dibuatin switch buat evolusi → buatin").

## AKAR yang dicabut (3 biji, Rule 5)
1. **Enrichment selalu nyuntik** — `maybeEnrichBrain` pakai `SemanticRetrieve` yang normalisasi
   skor ke top-hit (hit #1 = 1.0 walau query sampah) → top-K snippet disuntik TIAP call.
2. **Ga ada budget agregat** — tiap injector (knowledge/skill/insting/antibodi) punya cap sendiri,
   total gabungannya ga dijaga → worst-case prompt bengkak.
3. **Intent-gated pruning nyabotase prompt-cache** — `maybeFilterTools` mangkas tool per-QUERY
   (isi+urutan beda tiap turn). Cache Anthropic hash prefix `tools → system → messages` →
   tools berubah = SEMUA breakpoint miss = persona+history dibayar ulang tarif cache-write
   tiap call. Ini biang boros limit walau prompt-cache ON.

## SEAM (di file FROZEN, POLA B — default = perilaku lama, delete-test PASS)
| Seam | File frozen | Default |
|---|---|---|
| `enrichRetrieve(ctx,db,query,opts)` | `router/internal/router/brainenrich.go` | `brain.SemanticRetrieve` (lama) |
| `applyInjectShaper(ctx,req,settings)` | `router/internal/router/dispatcher.go` (+ dipanggil di `dispatcher_stream.go`) | no-op |

`applyInjectShaper` = titik tunggal pembentuk request PASCA semua injeksi+filter — ekstensi
masa depan (reorder cache-aware, dedup, dll) tinggal wrap di sibling, JANGAN buka frozen lagi.

## EXTENSION (sibling NON-frozen — bisa dihapus, inti tetap jalan)
| File | Isi | Switch (GUI fwswitch) |
|---|---|---|
| `router/internal/router/enrich_selective_ext.go` | retrieve pakai `SemanticRetrieveScored` (cosine ABSOLUT + lantai); 0 hit relevan → SKIP suntik. Index belum siap / error → fallback lama (fail-open) | `FLOWORK_ENRICH_MINSCORE` (float, 0=off, saran 0.30–0.45) |
| `router/internal/router/inject_budget_ext.go` | total char suntikan dikenal > budget → buang PESAN UTUH per-prioritas: knowledge(1) → insting(2) → antibodi(3). Doktrin SACRED + persona caller TIDAK PERNAH disentuh | `FLOWORK_INJECT_BUDGET` (char, 0=off, saran 6000–12000) |
| `router/internal/router/tools_sticky_ext.go` | union AKUMULATIF per-agent atas hasil pruning; urutan FIRST-SEEN append-only → prefix tools stabil → cache idup. Cuma aktif saat `FLOWORK_DYNAMIC_TOOLS` on | `FLOWORK_TOOLS_STICKY` (bool, default ON) |
| `router/internal/brain/vindex_ready_ext.go` | `VectorIndexReady()` — expose kesiapan index vektor buat fail-open | — |

Header suntikan yang dikenal budget (HARUS sinkron sama builder frozen):
`## Relevant knowledge` / `You are operating with a shared knowledge brain` / `## Applicable skills`
(brainenrich) · `## Insting —` (instinctenrich) · `## Antibodi —` (mistakeenrich) ·
`## Project doctrine` = SACRED (ga disentuh).

## BONUS FIX di file yang sama (2026-07-02)
- **Parity stream**: `dispatcher_stream.go` dulu GA nyuntik konstitusi (doktrin SACRED) di jalur
  utama (cuma di fallback) → chat streaming jalan tanpa doktrin. Sekarang gate-nya sama persis
  non-stream (`!isCrewLightModel` → `maybeInjectConstitution`).
- **BUG switch retry**: `FLOWORK_ROUTER_RETRY` terdaftar GUI sebagai bool padahal pembaca
  (mr-flow `main.go:866` + `agentkit.go:197`) baca INT jumlah-attempt (default 5) → toggle ON
  ("1") malah MATIIN retry. Registry dibetulin jadi int default 5 (+ nerve seed). Nilai live
  yang salah ("1") dibetulin ke "5".
- **BUG boot service**: `agent/start.sh` pid/log hardcode `/tmp/flowork-gui.*` → bentrok
  kepemilikan antar-user (mrflow manual vs service `flowork`) = service GAGAL boot. Fix:
  `RUN_DIR` per-user (`$XDG_RUNTIME_DIR|/tmp`/flowork-`$(id -un)`, override `FLOWORK_RUN_DIR`) +
  symlink kompat `/tmp/flowork-gui.{log,pid}` + port-in-use yang jawab HTTP = exit 2 idempoten
  (bukan failure). `stop.sh` ikut + fallback path legacy. Windows `.bat` ga kena (ga pake pid file).

## FILE-READ DEDUP (nyusul, sesi yang sama — gape1 §C "Tinggi TPM")
Seam `fileReadDedup` di `agent/internal/tools/builtins/file.go` (FROZEN, default no-op) +
`file_dedup_ext.go` (NON-frozen) + switch GUI `FLOWORK_FILE_DEDUP` (default ON).
Baca-ulang file GA-berubah (mtime+size sama, agent sama, ≤10 menit) → STUB: `unchanged:true` +
head 600 char + arahan pakai `{"force":true}` buat isi penuh. Anti-jebakan: stub SELALU bawa head
(hasil lama bisa udah ke-prune dari context mr-flow) + TTL + invalidasi mtime. Cache in-memory
per-proses, per-agent. Unit test 5/5 (`file_dedup_ext_test.go`). Delete-test PASS.

## F-E BREAKPOINT KE-4 — CACHE LINTAS-TURN (IMPLEMENTED 2026-07-02, owner-approved)
Akar lama: Tier-3 volatile mr-flow dikirim sbg system ke-2 → duduk ANTARA persona stabil
dan history di prefix cache → tiap turn prefix putus di situ → history ga pernah READ-hit.

Fix (2 file frozen di-unlock seizin owner → edit → re-hash → re-freeze):
1. **mr-flow `main.go`**: switch `FLOWORK_TIER3_TAIL` (GUI, default ON) — volatile Tier-3
   (waktu/memory/recall/tugas-aktif) dikirim sbg **pesan user TERAKHIR** (header
   `=== KONTEKS SISTEM OTOMATIS ===`), BUKAN system ke-2 → system 100% stabil + history
   dari DB byte-identik lintas-turn. OFF → perilaku lama persis. Forward env ke guest
   WASM via sibling `agent/envfwd_tier3_ext.go` (papan RegisterEnvForward, deletable).
2. **translator `tools.go`**: `markPrevMessageCache` — cache_control di block terakhir
   pesan KEDUA-terakhir = akhir prefix history STABIL (pesan terakhir = muatan volatile).
   Total breakpoint = 4 (max Anthropic): tools + system(persona) + prev-message + last-message.

Mekanika read-hit turn N+1: prefix [tools ✓, system ✓, u1..a(N-1) ✓ (bp prev-message
turn N)] KE-BACA dari cache; cuma [uN bersih + jawaban + ekor volatile baru] yang ditulis.
**Terukur** (claude-haiku, 2 turn bentuk mr-flow): turn-1 `read=0 create=6714` →
turn-2 `read=5876 create=866`. Live mr-flow: jam/memori/tool normal (D18-ctx utuh).
Catatan: turn yang kena `compressHistory` / suntikan router (enrichment fires) tetap
re-write history turn itu — by design (selective enrichment ngurangin frekuensinya).
Gemini nerima 2 pesan user beruntun (dites live); antigravity/openai path cuek sama
cache_control (cache implisit Gemini jalan sendiri).

## STATUS FREEZE
`brainenrich.go` / `dispatcher.go` / `dispatcher_stream.go` / `file.go` / `v9_extras.go`
di-unlock (FD LOCKBOX) → seam/fix → re-hash `KERNEL_FREEZE.md` → `chattr +i` lagi.
**Update 2026-07-02 (perintah owner): 5 file EXT implementasi ikut DIBEKUKAN** setelah
terbukti stabil + live-tested: `enrich_selective_ext.go`, `inject_budget_ext.go`,
`tools_sticky_ext.go`, `vindex_ready_ext.go`, `file_dedup_ext.go`.
JALUR EVOLUSI (tanpa buka file beku manapun): (a) switch GUI (fwswitch registry —
NON-frozen extension point), (b) sibling `_ext` BARU yang wrap/override seam yang sama —
`applyInjectShaper` & `enrichRetrieve` & `fileReadDedup` semuanya composable chain.
`TestKernelFreeze` PASS · gembok verified ("Operation not permitted") · delete-test PASS ·
unit test PASS · full `go test ./...` agent & router PASS.

## BUKTI LIVE (Rule 9, bahasa manusia)
mr-flow "coba liatin isi folder utama proyek" → tool jalan, jawab jujur, no muntah/loop.
Log router: `tools-sticky: union 1→14→15 tool (baru 0)` = urutan stabil lintas-iterasi.
