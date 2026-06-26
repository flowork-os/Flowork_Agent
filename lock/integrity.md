# Integrity — Anti-Tamper Mesh (root-hash frozen-core)

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com
> Standar freeze: lock/frozen-core.md. Switch: FLOWORK_INTEGRITY_GATE (default ON).

## Fungsi
Flowork didesain abadi: kalau frozen-core sebuah node pernah DIMODIFIKASI, node itu
**tak lagi dipercaya menyerap pembelajaran**. Dari SEMUA file frozen router → 1 root-hash
agregat (fingerprint). Kalau ada satu file frozen berubah/hilang → root beda → node **tampered**
→ gate mesh L0 **tolak semua knowledge masuk mentah-mentah**.

## Logic / Alur
- `mesh.CoreClean()` (FROZEN `internal/mesh/integrity.go`): baca manifest `KERNEL_FREEZE.md`
  (env `FLOWORK_KERNEL_MANIFEST` atau `../KERNEL_FREEZE.md`), hitung ulang sha256 tiap entri
  ber-prefix `../router/`, cocokin. Beda/hilang → `clean=false`. Hasil di-cache (`sync.Once`).
- `CoreRootHash()` = sha256 dari gabungan hash (terurut) = fingerprint core.
- **Gate** (FROZEN `internal/mesh/filter_integrity.go`): `init()` daftar lewat seam
  `RegisterMeshFilter` (filter_ext.go) → lapis **L0-core-integrity** di `RunFilterPipeline`.
  `!CoreClean()` → `Decision:"reject"` → `ProcessKnowledgePacket` drop (`StatusDropped`).
  Nol buka frozen: pakai seam + sibling file.
- **Degrade aman**: manifest tak ada (node shipped tanpa secret) → tak bisa verifikasi →
  `clean=true` (jangan blokir; andalkan `chattr +i` + rilis ber-tanda-tangan).
- **Self-protecting**: integrity.go + filter_integrity.go sendiri FROZEN & masuk manifest →
  ngedit checker = ubah root = ke-deteksi. `chattr +i` = tak bisa diedit MAUPUN dihapus
  tanpa unfreeze (sudo).

## File yang dilewati
- `router/internal/mesh/integrity.go` — CoreClean/CoreRootHash/CoreCheckedCount (FROZEN).
- `router/internal/mesh/filter_integrity.go` — gate L0 via RegisterMeshFilter (FROZEN).
- `router/internal/mesh/integrity_test.go` — bukti clean/tamper/absent (non-frozen test).
- `router/handlers_integrity_ext.go` — `GET /api/integrity` lewat RegisterExtraRoute (non-frozen).
- `router/internal/mesh/{filter_ext,karma_toolshare_filter,pipeline}.go` — pipeline 9-lapis (FROZEN).
- `flowork-secrets/KERNEL_FREEZE.md` — manifest privat (gitignored, symlink `../KERNEL_FREEZE.md`).
- `agent/internal/fwswitch/registry.go` — switch FLOWORK_INTEGRITY_GATE (GUI, non-frozen).

## Teknologi
- sha256 per-file + sha256-of-sorted-hashes (root). Pure-Go stdlib.
- Seam `RegisterMeshFilter` (init-append) → gate tanpa unfreeze.
- Ed25519 packet signing (mesh) = lapis L1 terpisah (anti-spoof); integrity = lapis L0 (anti-tamper-diri).

## Switch / batas
- `FLOWORK_INTEGRITY_GATE=0/false` → matiin gate (TIDAK disarankan; via GUI fwswitch).
- Cakupan v1 = **self-quarantine** (node tampered berhenti BELAJAR dari mesh). Atestasi root-hash
  antar-peer (tolak knowledge dari peer tampered) butuh field di Packet (FROZEN) → roadmap v2.
- Threat-model (sama `CARAFREEZE.MD`): ini tamper-EVIDENCE + auto-reject-belajar, BUKAN
  anti-root-jahat (root bisa unfreeze). Naikin palang, bukan tembok absolut.

## Status (QC live 2026-06-27)
- `/api/integrity` → `{checked:409, clean:true, root_hash, verified:true}` (manifest asli).
- Bukti tamper LIVE: 1 hash manifest dikorup → `clean:false` + root berubah → gate reject. Lulus.
- Unit `TestCoreIntegrityCleanAndTamper`: clean→true, hash-beda→false, manifest-absen→true. PASS.
