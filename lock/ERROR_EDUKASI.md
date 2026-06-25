# ERROR-EDUKASI — Cara Flowork Belajar dari Kesalahan (Guard, Recovery, Anti-Poison)

> Dokumen referensi KANONIK (white-label). Owner: Aola Sahidin (Mr.Dev).
> Update: 2026-06-25. ⚠️ Ke-track repo → NOL data personal owner.
> Prinsip owner: **"tiap kesalahan ada pembelajaran"** + **"deterministik = kuat, LLM lemah = rapuh"**.
>   → karena model lokal (~26B) lemah, guard-rail harus DETERMINISTIK di HARNESS, jangan ngarep model nyadar sendiri.

---

## 0. FILOSOFI + AKAR

Model lokal Flowork (~26B) jauh lebih lemah dari model-frontier triliunan. Konsekuensi (kebukti 2026-06-25):
dia **gak bisa self-correct** dari error/loop sendiri, dan **belajar pola SALAH** dari output-nya sendiri.
Jadi error-edukasi BUKAN cuma "simpan pelajaran" — tapi **HARNESS yang mungutin kesalahan + maksa koreksi +
nyaring apa yang boleh jadi pelajaran**. Tiga sumbu:

1. **CEGAH ngaco real-time** (in-loop guards): ghost-guard, flail-guard.
2. **PETIK pelajaran yang BENER** (post-turn learning): captureRecovery, edu-errors, dream-digest.
3. **TOLAK pelajaran yang SALAH** (anti-poison): anti-anchor (history) + digest-filter (graph).

**Prinsip kunci #3 (owner 2026-06-25):** reply agent = **HIPOTESIS, bukan ground-truth**. Jawaban-gagal/halu
JANGAN jadi pelajaran. Ground-truth = pesan USER + hasil TOOL, bukan tebakan model.

---

## 1. MEKANISME (urut: cegah → petik → tolak)

| # | Mekanisme | Kapan fire | Aksi | File | Status |
|---|---|---|---|---|---|
| A | **ghost-guard** | model NARASI niat tanpa manggil tool ("bentar gw cek...") | nudge paksa-aksi, bounded `maxGhostNudges=6` → kalau nunggu → ScheduleWakeup | `agents/mr-flow/main.go` (`looksLikeGhostPromise`, `ghostNudgeMsg`) FROZEN | live (lama) |
| B | **flail-guard** ⭐ | tool SAMA berulang TANPA progress (`file_list`×20) | nudge keras bounded `maxFlailNudges=4` → redirect (tool-lain/`tool_search`/`ScheduleWakeup`/kasih-hasil) → tetep mantok = **eskalasi JUJUR ke owner** | `agents/mr-flow/flail_guard.go` FROZEN + wiring `main.go` | **2026-06-25 (BARU)** |
| C | **captureRecovery** | tool ERROR lalu tool SAMA SUKSES dalam 1 turn | catat "WHEN <tool> <kelas-error> → recovered" → recovery-instinct (generalisasi privacy-safe) | `agents/mr-flow/recovery_capture.go` FROZEN | live (lama, D32) |
| D | **edu-errors** | tool gagal (kode error tertentu) | inject pelajaran statik by-Code (mis. tool-not-found → "pakai tool_search dulu") | `internal/agentdb/edu_errors*.go` (+ `edu_errors_ext.go` NON-frozen) | live (lama) |
| E | **anti-anchor (history)** ⭐ | reply-GAGAL/HALU ke-load balik dari history | `fetchHistory` BUANG reply-noise + user-Q pasangannya (biar model ga ngechо jawaban-buruk) | `main.go` (`isAnchorNoise`/`anchorNoisePhrases`) FROZEN + **`anchor_noise_ext.go` NON-frozen** | E-core lama; **fabricated-failure 2026-06-25 (BARU)** |
| F | **dream-digest gate** | interaksi di-digest ke cognitive-graph | `GateStatus` (confidence + antibody) saring node hasil-ekstrak | `internal/agentdb/cognitive_gate.go` + `cognitive_dream.go` FROZEN | live (lama) — **GAP: belum saring interaksi-GAGAL (lihat §4)** |

**Alur 1 turn (di mana tiap guard nempel):**
```
USER msg
  └─ tool-loop (main.go, TIME-BOUND ~200s, maxToolIters=100 backstop):
       ② call LLM → ③ tool_calls?
          ├─ TIDAK (teks) → [A ghost-guard] narasi-janji? → nudge & lanjut : else FINAL
          └─ YA → ④ runTool → ⑤ tempel hasil
                   ├─ [C captureRecovery] error→sukses? → recovery-instinct
                   └─ [B flail-guard] tool-sama-berulang? → nudge/eskalasi
  └─ reply di-LOG ke `interactions`
POST-turn / next-turn:
  └─ [E anti-anchor] fetchHistory buang reply-gagal/halu  ← anti-regurgitasi
  └─ [F dream-digest] interaksi → graph (gate confidence)  ← GAP: belum tolak interaksi-gagal
```

---

## 2. ⭐ FLAIL-GUARD (anti-mantok) — detail

**Akar:** mr-flow ditanya bahasa-manusia "cek perubahan repo" (pas tool `git` ga ke-expose) → manggil `file_list`
~20× (args kebanyakan SAMA) → **lolos SEMUA guard lain**: bukan narasi (ghost lewat), bukan ERROR (captureRecovery
lewat), masih < budget-waktu. Error-edukasi cuma nangkep ERROR, **gak nangkep FLAILING (sukses-tapi-sia-sia berulang)**.

**Deteksi = WINDOW-DUPLIKAT** (BUKAN "tool sama beruntun"): signature `tool|args` yang SAMA PERSIS muncul
**≥3× dalam 8-call terakhir** → flail. Nangkep repeat-beruntun (`job`×15) DAN cycling (`job/tools/log/job...`),
tapi **NOL false-positive** di kerja sah args-beda (baca 10 file beda = 10 sig unik → aman).

**Aksi (hormati owner "loop jangan dibatasi"):** BUKAN hard-stop. Koreksi keras bounded (`maxFlailNudges=4`) →
redirect. Tetep mantok lewat batas → **eskalasi JUJUR ke owner** (`flailEscalation`: "gw mantok di X, butuh arahan").

**Bukti:** 4/4 unit test (identik · cycling · no-false-positive · eskalasi-bounded) + tinygo build OK.

---

## 3. ⭐ ANTI-ANCHOR fabricated-failure (history-poisoning Layer-1) — detail

**Akar (history-poisoning):** model REGURGITASI jawaban-gagal-nya SENDIRI dari history. Kebukti live: pas mantok,
mr-flow NGARANG "error 503 service unavailable" + punt ScheduleWakeup → jawaban-halu itu masuk `interactions` →
di-feed balik tiap turn → di-ECHO terus TANPA beneran kerja. **Error-edukasi KEBALIK** — yang ke-reinforce malah ngaco.

**Fix:** extend mekanisme anti-anchor existing (yang udah buang reply-denial "nggak tahu") buat nangkep kelas
**fabricated-failure / malformed**. Pola di `anchor_noise_ext.go` (`init()`-append ke `anchorNoisePhrases`):
`error 503` · `503 (service` · `service unavailable` · `tool service` · `nolak koneksi gue` · `lagi down atau
overloaded` · `<tool_call>` (model nulis tool-call sbg TEKS = malformed). `fetchHistory` buang reply-match + user-Q
pasangannya → model ga anchor ke jawaban-buruk.

**Kenapa NON-frozen seam:** biar AI/owner nambah pola TANPA buka freeze main.go (konvensi `_ext`). Pola = spesifik
(minim false-positive); JANGAN masukin frasa umum.

---

## 4. KEPUTUSAN GW (rasional, biar AI lain ngerti)

- **flail-guard pakai window-duplikat, BUKAN streak-tool-sama.** Streak false-positive di batch sah (baca 10 file =
  10× file_read beruntun = SAH). Window-dup cuma nyala kalau sig IDENTIK berulang → sinyal flail bersih.
- **flail-guard ESKALASI ke owner, bukan hard-stop.** Hormati owner "loop jangan dibatasi (biar evolusi)". Stuck
  beneran → tanya owner (sepadan prinsip harness matang: escalate-when-genuinely-stuck), bukan ngarang/muter.
- **anti-anchor pakai NON-frozen seam (init-append), BUKAN edit main.go.** main.go hash UTUH → 0 unfreeze ceremony +
  extensible. Trade-off: phrase-based (band-aid-ish) — root sejati = §5 outcome-tagging deterministik.
- **flail_guard.go di-FREEZE, anchor_noise_ext.go TIDAK.** Logika flail = stabil/final (freeze). Pola anchor = tumbuh
  (growth-point non-frozen, konvensi `_ext`).
- **Akar history-poisoning = "reply agent itu hipotesis, bukan fakta".** Filter di sumber-baca (history) DULU karena
  itu yang OBSERVED. Graph permanen (digest) = §4-gap, belum.

---

## 5. SISA YANG BELUM KELAR (jujur — buat AI selanjutnya)

1. **Layer-2 DIGEST filter (PRIORITAS).** Layer-1 cuma channel HISTORY (transient, rolling). Interaksi-GAGAL masih
   bisa ke-digest jadi **node PERMANEN** di cognitive-graph (`DigestPendingInteractions`, `cognitive_dream.go` FROZEN).
   TODO: skip/tandai interaksi `outcome=failed`/anchor-noise sebelum digest. → lindungi graph permanen.
2. **Deterministic outcome-tagging (root sejati, > phrase-match).** Tag reply di `logInteraction` metadata:
   `outcome=failed` kalau turn lewat failure-path (flail-eskalasi / ghost-max / auto-continue-give-up). Lalu
   `fetchHistory` + digest filter by `outcome` (bukan nebak frasa). Sentuh main.go + cognitive_dream (frozen, ceremony).
3. **Akar FABRIKASI-error.** Kenapa model NGARANG "503" padahal ga ada? Sebagian ketutup flail-guard (ga mantok lagi)
   + deferred-tools (#2C — model ga buta tool). Tapi fabrikasi-saat-bingung belum dicabut tuntas.
4. **Live-validation flail-guard** di konteks BERSIH (history kepoison bikin run kemarin inconclusive — model
   regurgitasi, ga flail). Logika udah proven unit-test; integrasi live nunggu konteks bersih.
5. **Bersihin interaksi-503 yang UDAH kepoison** di `interactions` (opsional — anti-anchor udah nyaring saat baca,
   tapi entry-nya masih ada + bisa ke-digest sampe Layer-2 jadi).

---

## 6. ⚙️ STANDAR DEBUG mr-flow (WAJIB — biar AI lain ga bingung)

> Ringkas di sini; versi "rule emas" di `flowork-secrets/ruleemas.md`.

**Jalur:** chat mr-flow lewat **HTTP `/api/chat`** = jalur IDENTIK Telegram (channel-agnostic core `InvokeAgentMessage`).
```bash
curl -s -m 280 http://localhost:1987/api/chat -H 'Content-Type: application/json' \
  -d '{"text":"<prompt BAHASA MANUSIA>","agent":"mr-flow","user":"owner"}'
```
**WAJIB BAHASA MANUSIA, BUKAN bahasa-perintah.** Kebukti 2026-06-25: prompt gaya-perintah ("panggil tool git") bisa
JALAN, tapi prompt bahasa-manusia ("cek perubahan repo dong") malah NGE-TRIGGER BUG (model flailing/halu). Bug nyata
cuma kelihatan lewat bahasa-manusia (itu cara user beneran ngomong). Test pakai bahasa-perintah = false-sense-of-OK.

**Liat apa yang kejadian:** `tail -f /tmp/flowork-gui.log` (agent) → cari `tool_call:` `flail-guard:` `ghost-guard:`;
`tail -f /tmp/flowork-watchdog.log` (router) → cari `instinct: injected` `constitution:`. Loop full ~2-4 menit (model
lokal lambat) → pakai timeout besar / background.

**Awas history-poisoning saat test:** run gagal numpuk di `interactions` → model regurgitasi jawaban-lama. Test
berulang di skenario sama = hasil bisa "nyangkut" jawaban run sebelumnya, BUKAN perilaku fresh. Pakai prompt
ber-variasi / konteks bersih.

---

## 7. FILE MAP

**FROZEN (chattr+i + hash KERNEL_FREEZE.md):**
- `agents/mr-flow/main.go` — tool-loop + ghost-guard + isAnchorNoise + fetchHistory + logInteraction (wiring flail-guard).
- `agents/mr-flow/flail_guard.go` — logika anti-mantok (`flailState.check`).
- `agents/mr-flow/recovery_capture.go` — captureRecovery (error→sukses → recovery-instinct).
- `internal/agentdb/cognitive_gate.go` · `cognitive_dream.go` — digest gate (anti-halu confidence).

**NON-frozen (growth-point — JANGAN di-freeze):**
- `agents/mr-flow/anchor_noise_ext.go` — pola anti-anchor (tumbuh; tambah pola di sini).
- `internal/agentdb/edu_errors_ext.go` — konten edu-error (override DO-UPDATE).

**Data:** `agents/mr-flow/workspace/state.db` (interactions · cognitive_nodes · mistakes).
