# KV-CACHE — Keystone #3 (token/compute-efisiensi koloni)

> Owner: Aola Sahidin (Mr.Dev). Update: 2026-06-25. Prinsip: model lokal lemah → HARNESS angkat beban.
> Token-efisiensi = syarat hidup koloni (1000 semut). Lapis STATIK prompt IDENTIK lintas-call →
> llama.cpp simpen KV-cache prefix & pakai ulang → prefill di-SKIP → tiap call cuma proses bagian dinamis.

---

## STATUS

| Bagian | Status |
|---|---|
| **prompt-cache** (reuse prefix sama di slot sama) | ✅ default-ON (llama.cpp `--cache-prompt`) |
| **cache-reuse** (reuse prefix via KV-shifting walau ada chunk berubah) | ✅ **ENABLED + verified live** (`--cache-reuse 256`) |
| prompt ordering (statik di DEPAN) buat maksimalin reuse | 🟡 FAVORABLE (message-level confirmed); tool-schema perlu ukur |
| slot-save-restore (persist KV lintas-restart) | ⬜ tersedia (`--slot-save-path`), belum dipakai |
| `-np` parallel slots (multi-semut barengan) | ⬜ default auto; belum di-tune buat koloni |

---

## SWITCH (yg udah jalan)

`router/internal/localai/runtime.go` (NON-frozen growth-point, tempat semua perf-flag llama-server):
```
ENV FLOWORK_CACHE_REUSE=N   → append "--cache-reuse N" (N = min chunk size, mis. 256)
                              "0"/"off"/kosong = TIDAK ditambah (eksplisit/ default mati)
```
**OPT-IN (default OFF di kode)** — sengaja: llama.cpp lama bisa ga kenal `--cache-reuse` → startup gagal.
Aktifin cuma di mesin yg llama-server-nya support (cek: `router/bin/llama-server.real --help | grep cache-reuse`).
Mesin ini: di-set `FLOWORK_CACHE_REUSE=256` di `router/flowork.local.env` (gitignored, machine-local).

**Sepasang** sama flag perf lain di file sama (pola ENV-gated): `FLOWORK_NGL`, `FLOWORK_CPU_MOE`,
`FLOWORK_KV_TYPE`, `FLOWORK_CTX`. Semua opt-in via `flowork.local.env`.

## KENAPA INI AKAR (bukan tambal)

Diagnosis boros (roadmap): prompt ~15.8k/turn, biang = tool-schema STATIS (~55%) + konstitusi (~13%) = ~68%
STATIK & IDENTIK tiap call. cache-reuse bikin prefix statik itu di-reuse (KV-shifting) → prefill-nya di-SKIP,
bukan dihitung ulang tiap turn. Output-transparent (optimasi prefill, BUKAN ubah sampling) → aman.

## VERIFIKASI

```
ps aux | grep llama-server.real | grep -- '--cache-reuse'      # harus muncul "--cache-reuse 256"
```
Verified live 2026-06-25: flowork-brain :8088 jalan dgn `--cache-reuse 256`, `/api/chat` default → HTTP200
balas koheren (no regression). Reversible: hapus/0-kan ENV → balik perilaku lama (tanpa rebuild kode).

## NEXT (sisa keystone #3 — belum dikerjain)

1. **Prompt ordering** (🟡 sebagian terverifikasi): di `dispatcher.go` urutan enrich = `maybeInjectConstitution`
   (STATIK, "di ATAS knowledge", dipanggil PERTAMA) → `maybeEnrichBrain` → `maybeInjectAntibodies` →
   `maybeInjectInstinct` (DINAMIS, by-query) → pesan user. Jadi message-level udah FAVORABLE (statik konstitusi
   di depan, dinamis recall/insting di belakang) → cache-reuse reuse prefix statik. **SISA:** konfirmasi posisi
   render TOOL-SCHEMA (biang ~55% statik) lewat jinja template — kalau di depan = reuse penuh; ukur prompt-eval
   time call-1 vs call-2 (prefix sama) buat bukti empiris. JANGAN ubah urutan sembarangan (persona FROZEN).
2. **slot-save-restore** (`--slot-save-path`): persist KV ke disk → warm-start lebih cepat lintas-restart.
3. **`-np` parallel slots**: tune buat banyak semut jalan barengan (visi 1000 agent) vs RAM/VRAM.
4. **Ukur**: warm 54s pecah prefill-vs-gen; recall@latency sebelum/sesudah cache-reuse pada beban nyata.
