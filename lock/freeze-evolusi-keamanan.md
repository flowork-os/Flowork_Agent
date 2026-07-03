# ❄️ FREEZE — Mesin Self-Evolve + Gerbang Keamanan + OS Builder

> Tujuan: Flowork boleh berevolusi otonom TANPA bisa melucuti guard-nya sendiri. Semua file di sini =
> MEKANISME/GUARD inti (kategori §5.2 rule emas) → di-FREEZE langsung, **tidak** di-switch. Alasan:
> pertumbuhan sudah ngalir lewat seam yang ada (`RegisterFeature`/`RegisterScanRule`/saluran nerve di
> [[peta-saraf]]) + jalur additive `NEW:` di `selfevolve_coreapply.go`. Bekuin engine ≠ nutup evolusi:
> evolusi = NAMBAH file sibling, bukan ngedit engine. Manifest: `flowork-secrets/KERNEL_FREEZE.md`
> (32 entri, section "FREEZE 2026-07-03"). Enforcement: `agent/freeze_test.go` + `chattr +i`.

## Kenapa file-file ini WAJIB beku (kalau editable = doktrin mentah)

- **`selfevolve_coreapply.go`** — semua guard hidup di sini: additive-only (`NEW:` doang), anti
  path-traversal (`evolveSafeRepoPath`), anti-timpa file aktif, cek LOCKED, gate ruang-saraf
  (`NerveProposalVet`), anti-RCE (sengaja TIDAK `go test` di sandbox). 1 edit = semua guard copot.
- **`selfevolve_push.go`** — auto-push token; editable = exfil token / push jahat ke semua user.
- **`internal/scanapi/scan_exec.go`** — SATU-SATUNYA gerbang exec-OS scanner (blocklist `rm/dd/sh/...`
  hardcoded). Editable = RCE penuh via 1 POST.
- **`guardian_handler.go`** — proteksi-diri (boot-check tamper→safe-mode, auto-arm).
- **`nerve_proposal.go` + `nerve_butuh_tombol.go` + `nerve_seed_ext.go`** — gate ruang-saraf yang
  dipanggil guard coreapply. `Allowed=true` paksa = gate lumpuh. (Ket: `_seed_ext` = seed katalog
  saraf, bukan sibling buang-an — lihat [[peta-saraf]].)
- **Klaster evolusi** `selfevolve.go`, `selfevolve_apply.go`, `selfevolve_stage.go`,
  `selfevolve_schedule.go`, `selfevolve_group_seed.go`, `evolve_capability.go`, `evolve_council.go`,
  `evolve_council_group.go`, `internal/agentmgr/selfevolve.go` — tiap file = 1 lapis gate autonomi.
- **Install pipeline** `plugin_handler.go`, `plugin_watcher.go`, `plugin_admin.go`, `tool_install.go`,
  `slash_install.go`, `pack_extract.go`, `verifier.go` — vector loading kode (caps-consent + anti
  zip-slip + gate deploy adversarial). Guard udah matang → dikunci.
- **Seed brain/persona** `ai_studio_brain.go`, `ai_studio_seed.go`, `internal/agentmgr/provision_dna.go`,
  `internal/agentdb/selfknowledge_seed.go` — nyuntik doktrin/DNA ke brain (Rule #8: haram ubah brain).
- **DDL stabil** `internal/floworkdb/scanner_registry.go`, `internal/floworkdb/scan_findings.go` —
  1 tabel append-only; nambah scanner = DATA row, bukan edit schema.
- **`os/builder/main.go` + `os/flasher/main.go`** — GUI flash USB destruktif (`dd`), builder bake
  secret `~/.flowork`. Copot `isFlashSafe()`/`sameSiteGuard()` = nuke disk / drive-by. (Ngaku LOCKED
  di header tapi dulu kelewat dari manifest.)
- **`router/internal/mitm/handlers/antigravity_capture.go`** — megang `Bearer` token live; +3 baris = exfil.

## File mesh (di luar KERNEL_FREEZE.md)

- **`router/internal/mesh/policy_export.go`** — wrapper editable yang ekspor gate consent mesh dari
  `policy.go` (beku di super_scrit). Ganti jadi `return true` = bypass `FLOWORK_MESH_SHARE`/approve.
  Status: **chattr +i (fisik) SUDAH**. Belum masuk manifest apa pun (Rule §6.1: file mesh haram di
  KERNEL_FREEZE.md). **TODO OWNER:** masukin hash ke `super_scrit.md` master di flashdisk biar
  ke-cover drift-detection mesh. Hash: `eb2039337de687822ab22d5f044d3d92c9fbad4c9539f3a64e82ed42214da75e`.

## DITAHAN (belum beku — butuh switch dulu, per §5)

- **`internal/agentdb/evolution.go` + `internal/floworkdb/chatdb.go`** — punya rekam jejak `ALTER TABLE
  ADD COLUMN` (tabel yg tumbuh-kolom). Bekuin sekarang = maksa AI masa depan buka file beku tiap nambah
  kolom (persis kasus §5 yg dihindari). **Butuh SEAM MIGRASI (registry migrasi additive) DULU**, baru beku.
- **`coder.go` + `architect.go`** — generator agent/app; udah dijaga hilir oleh `verifier.go` (beku).
  Opsional beku nanti; ditahan biar generator masih boleh berkembang.

## Catatan arsitektur (cabut akar, bukan tambal)

- **Sentinel runtime buta file root.** `internal/guardian/guardian.go:238` regex `(internal/[^\s]+\.go)`
  → tamper-check saat boot cuma nge-hash file `internal/`. File `package main` di root `agent/` (termasuk
  engine self-evolve yg baru dibeku) HANYA dijaga `chattr` + `freeze_test`, TIDAK oleh guardian runtime.
  Fix akar: longgarkan regex biar cover root (guardian.go beku → butuh unfreeze sadar).
- **`start.sh` auto-pull tanpa signature** = benteng sebenarnya buat user (chattr TIDAK ikut git → user
  hasil `git pull` file-nya editable). Fix akar: verify manifest hash + signature (`os/config/update-pub.pem`
  udah ada) + pin tag signed sebelum rebuild — samain postur dgn jalur OS-image. **Belum dikerjain.**

## QC (2026-07-03)
build agent+router ✅ · vet ✅ · unit test ✅ · TestKernelFreeze PASS (823 file) ✅ ·
gembok aktif (Operation not permitted) ✅ · delete-test agent+router build OK ✅ · tes mr-flow bahasa manusia ✅
