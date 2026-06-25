# AGENTKIT — Kit Bersama "Pasukan Semut" (loop tool-calling + guard ter-ekstrak)

> Dokumen kanonik (white-label). Keystone roadmap koloni: hapus duplikasi loop worker
> 6× → 1 package SHARED → semua semut warisan guard. Owner: Aola Sahidin (Mr.Dev).
> Repo: https://github.com/flowork-os/Flowork-OS. Update: 2026-06-25.

---

## ⛔ WAJIB BACA DULU

- **`agent/agentkit/`** = modul Go lokal `flowork-agentkit` (tinygo-safe). Ini **inti loop**
  semua worker-agent. Edit di sini = **rebuild SEMUA agent** (agent = WASM terpisah, share
  SOURCE compile-time). NON-frozen (growth-point), tapi hati-hati: rusak guard = 26B ngamuk.
- **mr-flow TIDAK pakai agentkit** — dia punya loop sendiri (`agents/mr-flow/main.go`, FROZEN,
  brain-heavy: Telegram I/O, media, recall, history, working-set). Sengaja TIDAK di-migrate
  (bukan akar duplikasi worker + buka jantungnya = risiko tanpa untung). mr-flow = referensi.

---

## 0. AKAR MASALAH (kenapa AGENTKIT ada)

Roadmap (Rule 5/6 cabut-akar): loop tool-calling tiap worker-agent KE-COPY. Guard
deterministik (flail/ghost) cuma lengkap di mr-flow → **5 agent lain + template GAK punya
flail-guard + GAK punya seam #2C** → nyalain all-tools/#2C buat mereka = **FLAIL**. Visi
1000-semut: gak mungkin maintain 1000 copy loop. **Solusi:** ekstrak loop+guard jadi 1
package SHARED → tiap `main.go` agent jadi **bootstrap TIPIS** → fix sekali, semua warisan.

**Hasil:** dari **6 copy loop** → **2 implementasi**: (1) `agentkit` (semua worker + template +
semut baru), (2) `mr-flow` (brain owner). all-tools/#2C **GLOBAL** ke-unblock buat semua worker.

---

## 1. ARSITEKTUR

```
agent/agentkit/                 ← modul `flowork-agentkit` (tinygo-safe, NON-frozen)
   ├── go.mod                   (module flowork-agentkit; go 1.23)
   ├── agentkit.go              Main() + tool-loop + #2C seam + host wasmimport + helpers
   └── guards.go                flail-guard + ghost-guard + recovery-capture

agents/<worker>/                ← BOOTSTRAP TIPIS (browse-surfer/-reporter/fbspecial/
   ├── main.go                    fb-writer/fb-repofinder): `func main(){ agentkit.Main() }`
   ├── go.mod                     (require + `replace flowork-agentkit => ../../agentkit`)
   └── manifest.json            (id, caps — yang bikin tiap semut beda)

templates/agent-template/       ← CETAKAN semut baru (FROZEN chattr; di-migrate 2026-06-25)
   main.go + go.mod             sama: bootstrap tipis + replace ../../agentkit
```

**Kenapa modul terpisah + `replace` relatif:** tiap agent = **modul Go sendiri** (biar tinygo
cuma compile dir itu, gak narik modul host `flowork-gui` yg gede). Share lewat modul lokal
`flowork-agentkit` + `replace => ../../agentkit` (relatif, valid dari `agents/*` DAN
`templates/agent-template/` — dua-duanya depth-2 di bawah `agent/`). **0 hardcode** (Rule 6).

---

## 2. APA YANG DI-SHARE (parity dgn mr-flow yang PROVEN)

| Bagian | Asal | Catatan |
|---|---|---|
| **Tool-loop** (LLM→tool→feed→ulang, serialize 1 tool/iter, `parallel_tool_calls:false`) | worker loop proven | inti |
| **GHOST-GUARD v2** (narasi niat tanpa tool → nudge **+ paksa `tool_choice:"required"` di req berikut**, bounded `maxGhostNudges=6`; nudge habis → **honest-fallback** ("tool ga kepanggil, model lokal ngeyel — coba ulang"), BUKAN return ghost-promise) | port mr-flow (phrase superset) | anti-ghosting **stabil** (2026-06-26) |
| **FLAIL-GUARD** (tool SAMA berulang tanpa progress → koreksi bounded → eskalasi jujur) | port `flail_guard.go` (proven 4/4) | **yang dulu KURANG di worker** |
| **#2C deferred seam** (`tool_lookup` → re-fetch specs; host-gated, no-op kalau defer off) | port mr-flow §tools.md 7.5 | **yang dulu KURANG di worker** + guard nil (lebih aman dari mr-flow) |
| **RECOVERY-CAPTURE** (error→sukses tool sama → `mistake_log`) | port `recovery_capture.go` | best-effort, graceful kalau tool ga ada |
| **TIME-BOUND + AUTO-CONTINUE** (`loopBudgetMs`/`ScheduleWakeup`, anti-runaway `maxAutoContinue=50`) | worker loop | unbounded lintas-turn |
| **ROUTER-RETRY** (exp-backoff transient, switch `FLOWORK_ROUTER_RETRY`) | worker loop | resilience |
| **stderr log** `tool_call:`/`ghost-guard:`/`flail-guard:` | tambah baru | observability Rule-9 (dulu worker ga log) |

**TIDAK dipasang di agentkit (sengaja):**
- **anti-anchor** (anti-503-regurgitasi): loop worker rakit `msgs` **FRESH** tiap turn (ga feed
  history balik) → ga ada regurgitasi-history. N/A. Kalau kelak worker dikasih history → nyusul.
- **recall_gate / working_set / auto-recall**: itu fitur brain mr-flow (history+memori). Worker
  loop ga auto-recall. N/A.

---

## 3. CARA NAMBAH/UBAH (jalan evolusi — tanpa buka frozen)

- **Filtur loop baru / tuning guard** → edit `agent/agentkit/{agentkit,guards}.go` → **rebuild
  SEMUA agent** (lihat §4). Itu memang tujuannya: 1 edit → semua semut dapet.
- **Semut baru** → `scripts/mk-agent.sh <id> <model> "<persona>" [tools-csv]` (copy template →
  sed module+`go 1.23` → build → provision). Otomatis warisan agentkit (replace ke-copy).
- **Semut existing yg masih loop LAMA** (deploy sebelum 2026-06-25) → rebuild dari source-nya
  (kalau ada di `agents/`) atau re-spawn via mk-agent. Mereka tetep JALAN (loop lama fungsional,
  cuma belum punya flail-guard/#2C) sampai di-rebuild.

---

## 4. BUILD & DEPLOY

```
cd agent
GOWORK=off GOTOOLCHAIN=go1.23.4 bash scripts/build-agent.sh <worker-id>   # tinygo → stage ke ~/.flowork/agents/<id>.fwagent/
```
- Reload: overwrite `agent.wasm` → fsnotify watcher kernel (debounce 1500ms) auto-load. Paksa:
  `mv <id>.fwagent /tmp/x && sleep 2 && mv /tmp/x <id>.fwagent`.
- agentkit compile di **tinygo (worker)** DAN **standard-go wasip1** (cross-checked `go vet` PASS)
  → kalau kelak mau, mr-flow pun bisa adopsi tanpa ganti toolchain.

---

## 5. FREEZE (owner-approved 2026-06-25 — "freeze lalu kunci di KERNEL_FREEZE.md")

- `agent/agentkit/agentkit.go` + `agent/agentkit/guards.go` = **FROZEN** (chattr +i + hash di
  `KERNEL_FREEZE.md` "SHA256 brain-core", **di-enforce TestKernelFreeze**). `agentkit/go.mod` =
  chattr +i (non-`.go` → manual-verify sha256sum di history-note). Inti loop SEMUA semut → edit =
  **SADAR**: `sudo chattr -i agentkit/*.go agentkit/go.mod` → edit → **rebuild SEMUA agent**
  (`bash scripts/build-agent.sh <id>` per worker) → re-hash sha256 → update KERNEL_FREEZE → `sudo
  chattr +i` → **verify TestKernelFreeze PASS + Rule-9**.
- `templates/agent-template/main.go` = **FROZEN** (chattr +i + hash di KERNEL_FREEZE, di-enforce —
  re-hash `3899790f…`→`4c773161…` pasca-migrasi bootstrap). `agent-template/go.mod` = chattr +i
  (non-`.go`, manual-verify).
- `agents/mr-flow/main.go` (+ guard mr-flow) = FROZEN brain-core (di KERNEL_FREEZE) — **JANGAN**
  disentuh buat agentkit (cuma di-BACA buat porting; git nyatat 0 ubah).
- **Pola nano-modular:** mekanisme STABIL beku (agentkit core), daftar identitas/persona/tool =
  RUNTIME (manifest + GUI/kv). Nambah fitur worker = idealnya tambah lewat config/host, BUKAN
  unfreeze loop (Rule 7). Unfreeze loop cuma kalau emang akar.

---

## 6. VERIFIKASI (2026-06-25 — Rule-9 bahasa-manusia)

`browse-surfer` (migrated, di-reload) di-invoke via `/api/kernel/rpc`
(`plugin:browse-surfer, function:handle_message`) pakai **bahasa-manusia** ("bro, coba intip
folder kerja lo ada file apa…"):
- ✅ Loop agentkit JALAN live (log `[browse-surfer] tool_call: file_list…/Glob…` — marker BARU).
- ✅ Jawaban koheren (nemu 2 file `job/run-*.md`, ringkas, nawarin baca isi).
- ✅ **NOL flail false-positive** (args beda → flail-guard bener ga trigger), NOL ghosting, NOL panic/wasm-trap.
- 5 worker + template build OK (tinygo, 282086b identik = bukti share-source deterministik).

---

## 7. SISA (follow-up, BUKAN blocker keystone)

1. **Rebuild semut deploy lama** ke agentkit (≈30 instance `~/.flowork/agents/*.fwagent` dari
   template lama: evo-*, stock-analyst-*, dll). Banyak ephemeral (tes-sinkron/uji-temp). Mereka
   tetep jalan; rebuild pas perlu flail-guard/#2C global.
2. **all-tools GLOBAL** sekarang AMAN dinyalain per-worker (seam #2C udah ada di agentkit) —
   pasangan wajib: perkuat insting/konstitusi (kemudi pilih dari ~200 tool) = roadmap #2/#2B.
3. **mr-flow adopsi agentkit?** OPSIONAL (DRY murni). Cross-compile udah kebukti. Butuh ACC owner
   + unfreeze main.go — tunda sampai ada untung nyata (sekarang mr-flow proven, jangan diutak-atik).
