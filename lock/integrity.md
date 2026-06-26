# Integrity — Anti-Tamper Mesh (2-tier, root-hash, anchor)

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com
> Switch: FLOWORK_INTEGRITY_GATE (default ON). Lihat juga: lock/mesh-sharing.md.

## 2 tingkat keamanan
- **Tier-1 (anti-edit):** `chattr +i` + manifest `KERNEL_FREEZE.md` (enforce `TestKernelFreeze`).
  Semua file frozen tak bisa diubah AI lain tanpa sadar. Privat (gitignored, di luar repo).
- **Tier-2 (JANTUNG mesh-trust):** manifest `flowork-secrets/super_scrit.md` (PRIVAT, di luar repo,
  gitignored, `chattr +i`). **29-32 file jalur trust mesh** (sign/packet/pipeline/filter/integrity/
  knowledge/karma/consensus/ingest/handler-mesh). Kalau salah satu BERUBAH → node **tampered** →
  gate L0 **tolak semua pembelajaran mesh**.

## Logic / Alur
- `mesh.CoreClean()` (FROZEN `internal/mesh/integrity.go`): `manifestPath()` = env
  `FLOWORK_KERNEL_MANIFEST` → `../super_scrit.md` (kalau ada) → `../KERNEL_FREEZE.md`. Baca entri
  ber-prefix `../router/`, hitung ulang sha256, cocokin. Beda/hilang → `clean=false`. Cache `sync.Once`.
- **Anchor (super_scrit bisa dilepas dari PC):** kalau manifest TAK ada → `computeFromAnchor()`
  (FROZEN `integrity_anchor.go`): hash file `tier2AnchorFiles`, banding root ke const `tier2AnchorRoot`
  (embedded di binary). File berubah → root beda → tampered. Const placeholder/kosong → fail-open aman.
- `CoreRootHash()` = sha256(gabungan hash terurut) = fingerprint tier-2.
- **Gate** (FROZEN `internal/mesh/filter_integrity.go`): seam `RegisterMeshFilter` → lapis
  **L0-core-integrity** di `RunFilterPipeline`. `!CoreClean()` → `reject` → drop (`StatusDropped`).
- **Self-protecting:** integrity.go + filter_integrity.go masuk tier-2 + `chattr +i` → ngedit checker
  = ubah root = ke-deteksi, dan tak bisa diedit/dihapus tanpa sudo. integrity_anchor.go = tier-1 (chattr+i).

## File yang dilewati
- `router/internal/mesh/{integrity,integrity_anchor,filter_integrity,policy,filter_meshpolicy}.go` (FROZEN).
- `router/handlers_integrity_ext.go` — `GET /api/integrity` (RegisterExtraRoute).
- `flowork-secrets/{KERNEL_FREEZE.md (tier-1), super_scrit.md (tier-2)}` — PRIVAT, di luar repo, chattr+i.
- `agent/internal/fwswitch/registry.go` — switch FLOWORK_INTEGRITY_GATE.

## Teknologi
- sha256 per-file + sha256-of-sorted-hashes (root). Pure-Go. Ed25519 packet signing = lapis L1 (anti-spoof) terpisah.

## Batas / threat-model
- super_scrit.md: **tak ke-upload** (gitignored, di luar repo `FLowork_os`), **immutable** (chattr+i),
  bisa **dilepas total dari PC** (anchor jaga). Owner bisa simpan di USB → set `FLOWORK_KERNEL_MANIFEST`.
- Sama `CARAFREEZE.MD`: tamper-EVIDENCE + auto-reject-belajar, BUKAN anti-root-jahat (root bisa unfreeze).
- v1 = self-quarantine (node tampered berhenti BELAJAR). Atestasi root antar-peer = roadmap.

## Status (live 2026-06-27)
- `/api/integrity` → `{checked:32, clean:true, root_hash, verified:true}` (super_scrit). Anchor: normal-root = const (terverifikasi).
- Tamper LIVE: 1 hash dikorup → `clean:false`. Unit `TestCoreIntegrityCleanAndTamper` PASS.
