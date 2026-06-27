# 🏭 AI STUDIO — Pabrik Kemampuan + Gerbang Pemeriksaan (rangkuman fase)

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com (white-label)
> Master rencana: `ROADMAP_AI_STUDIO.md`. Arsitektur inti: `lock/ARSITEKTUR.md`. Adopt: `lock/apps-adopt.md`.
> Status: ✅ F1-F4 SELESAI + build/vet/TestKernelFreeze PASS (2026-06-27).

## Inti
AI Studio = **PABRIK** yang BIKIN kemampuan baru (tool/agent/app/scanner/channel/slash) lewat **1 GERBANG
PEMERIKSAAN per-jenis** + kelola siklus-hidupnya (lahir → periksa → sehat/sakit → mati). Beda dari **mr-flow**
(tukang yang PAKE kemampuan). Studio = TANGAN self-evolution: pas inti beku, satu-satunya cara nambah kemampuan.

## Pipeline (yang udah jalan)
`Coder` (coder.go: ide→spec LLM→rakit .fwpack) → `Verifier` (gerbang per-jenis) → owner-approve →
`installPluginPack` (gerbang 6-jenis) → smoke. `Reaper` (reaper.go) scan kesehatan, owner buang. `Death-Letter`
catat kematian.

## F1 — Beda Studio vs mr-flow (label GUI) ✅
- Header tab AI Studio (`web/tabs/coder.js`) di-label tegas: 🏭 **PABRIK KEMAMPUAN** (bikin+periksa+kelola) vs
  **mr-flow** (tab Chat = kerjain tugas). Icon 🧬→🏭. Nol bingung.

## F2 — Pemeriksa per-jenis + gerbang WAJIB (cabut-akar) ✅
- **`verifier_apptype_ext.go`** (sibling NON-frozen): `verifyCapability(kind, raw)` dispatch per-jenis —
  - agent/category (kind kosong) → `verifyPackStatic` LAMA (agent.wasm + persona + 1 synth). TAK diubah.
  - `app` → `verifyAppPack`: SKIP wasm/persona; cek zip+manifest(kind=app) + consent-exec + scan pola-jahat.
  - tool/slash/scanner/channel → `verifyGenericPack`: zip+manifest+scan, tanpa maksa wasm.
- **REUSE scanner FROZEN** `adopt.ScanRepo` (`internal/apps/adopt/scan.go`): app .fwpack di-extract ke temp-dir →
  ScanRepo → cleanup (`scanPackBytes`). 1 sumber kebenaran pola berbahaya (sama dengan adopt repo).
- **Gerbang WAJIB:** `installPluginPack` (plugin_handler.go) panggil + ENFORCE `verifyCapability` di pucuk (sebelum
  dispatch kind). verdict `blocked` → TOLAK (403), kecuali owner `overrideBlocked` SADAR. Param baru
  `overrideBlocked` (KEPISAH dari `approveCaps`: izin caps ≠ paksa pola-jahat). 5 caller di-update
  (HTTP install, coder-approve, architect, drop-folder, watcher). Nutup 2 lubang lama: app (dulu ga pernah
  diverify) + direct-install (dulu cuma advisory).
- **Bukti:** `verifier_apptype_ext_test.go` — app jahat (reverse-shell/rm-rf) → blocked; app bersih → review
  (consent exec); tool jahat (pipe-shell) → blocked; jalur app TIDAK ngecek agent.wasm (cabut-akar).

## F3 — Panel Siklus Hidup + Death-Letter ✅
- **`deathletter_ext.go`** (sibling NON-frozen, DELETABLE): `recordDeathLetter` → `~/.flowork/death-letters.json`
  (cap 200, terbaru dulu, best-effort). `GET /api/studio/deathletters` (route via `RegisterFeature`).
  Hook di `uninstallCategoryCore` (plugin_admin.go, +param `reason`) → SEMUA pembuangan (reap & manual) tercatat.
- **`web/tabs/studio_lifecycle.js`** (panel di tab AI Studio): 1 layar gabung 3 sumber yang udah ada —
  `/api/coder/pending` (nunggu approve + verdict Verifier), `/api/reaper/candidates` (kesehatan sehat/sakit/mati),
  `/api/studio/deathletters` (surat kematian). Tombol Setujui/Tolak/Buang (owner yang mutusin).
- **Bukti:** `deathletter_ext_test.go` (record→list round-trip, terbaru-di-depan).

## F4 — Studio jadi 1 pintu (adopt + evolusi) ✅
- **EVOLUSI:** self-evolution `architect.go` install lewat `installPluginPack` → kena gerbang `verifyCapability`
  yang SAMA (override=false: AI ga boleh paksa pola-jahat ke dirinya).
- **APP/adopt:** `adopt_ext.go` pre-flight `adopt.ScanRepo` di-log `[ai-studio gate] repo-adopt verdict=...` →
  tiap adopt keliatan lewat gerbang Studio. Scanner SAMA dengan verifyAppPack (folder vs zip, 1 sumber pola).

## Verifikasi (2026-06-27)
`go build ./...` =0 · `go vet ./...` =0 · `TestVerify*`/`TestDeathLetter*` PASS · `TestKernelFreeze` PASS ·
adopt/cliadapter/httpadapter tests PASS · JS `node --check` OK. (TestFlowAlphaApp = test integrasi live LLM/network,
timeout di sandbox — bukan regresi.)

## ❄️ FREEZE + SWITCH (2026-06-27 — gate stabil dikunci, tetap extensible)
Gerbang + siklus-hidup yang udah stabil DIBEKUKAN (chattr +i + hash di KERNEL_FREEZE.md) biar AI/asisten masa
depan ga ngubah TANPA SADAR. 2 file CORE frozen, masing-masing self-contained (lolos delete-test):
- **`studio_gate.go`** (FROZEN) — VerifyCheck/VerifyVerdict/finalizeVerdict + `verifyCapability` dispatcher +
  `verifyAppPack`/`verifyGenericPack` + `scanPackBytes` (REUSE adopt.ScanRepo frozen). Self-contained: dep cuma
  stdlib + paket adopt (frozen). Regex id sendiri (`gateIDRe`), ga gantung simbol non-frozen.
- **`deathletter.go`** (FROZEN) — store surat kematian + handler. Dep cuma stdlib + loader/tfWriteJSON/
  feature_registry (semua frozen).

3 SWITCH (Rule #7 — freeze WAJIB bikin switch dulu) biar tetap tumbuh tanpa buka freeze:
- **`RegisterCapabilityVerifier(kind, fn)`** (POLA A, di studio_gate.go) — jenis kapabilitas BARU (mis. "workflow")
  diperiksa lewat sibling `init()` tanpa nyentuh gate.
- **`var verifyAgentPack`** (POLA B, di studio_gate.go) — jalur AGENT kaya (manifest+persona+wasm) hidup di
  `verifier.go` (NON-frozen), di-pasang ke gate via `init(){ verifyAgentPack = verifyPackStatic }`. Hapus
  verifier.go → gate fallback ke cek AMAN minimal, **build tetap 0** (self-sufficient).
- **`RegisterDeathObserver(fn)`** (POLA A, di deathletter.go) — reaksi-pada-kematian BARU (broadcast mesh, notif).

NON-frozen (tetap growth surface): `verifier.go` (agent verify + LLM-judge), `plugin_handler.go` (dispatch 6-jenis
yang tumbuh), `coder.go`/`architect.go`/`reaper.go`/`plugin_admin.go` (call-site gerbang), `studio_lifecycle.js`
(GUI). Scanner `scan.go` cuma di-REUSE (udah frozen sejak apps-adopt).
