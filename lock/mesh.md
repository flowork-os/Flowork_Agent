# MESH — jaringan P2P antar-node (GUI 🕸️ Mesh & Policy)

> Dev: Aola Sahidin. 2026-06-26. Mesh = node Flowork saling kenal & tukar pengetahuan/tool TANPA
> server pusat (mDNS + rendezvous, paket bertanda-tangan ed25519, filter 9-lapis). **PHASE-1**
> (skema + seed lokal single-owner; traffic multi-host real = phase-2). Cara kerja: `os/`.

## FITUR (console Section 13-27)
Identity (ed25519) · Peers (13) · Signed Packets (14) · Knowledge Inbox (17) · Tool Manifests (18) ·
Peer Karma+decay (19) · **Filter Pipeline 9-lapis (20)** · Provider Chains (24) · LocalAI Runtime (25) ·
Pricing (26) · Policy/Budget (27). Internal: CRDT, gossip, discovery, consensus N-of-M, blocklist, LoRA-delta.

## JALUR FILTER 9-LAPIS (inti keamanan mesh)
Paket pengetahuan masuk dari peer → `ProcessKnowledgePacket` → `RunFilterPipeline` (karma_toolshare_filter.go):
```
L1-signature (ed25519 verified) → L2-freshness (≤24h, no future) → L3-karma (reputasi origin) →
L4-quarantine (substring poisoning) → L5-pii (strip) → L6-injection (prompt-injection) →
L7-cosine (near-dup vs promoted) → L8-consensus (N-of-M endorsement) → [SEAM] → L9-promote
```
Reject di lapis manapun = paket ditolak (return early). Verified live: test "ignore previous
instructions…" → L1 pass, L2 reject → final_pass=false.

## FROZEN — logic (24 file, chattr +i + KERNEL_FREEZE + TestKernelFreeze)
`internal/mesh/*` (17: blocklist/consensus_phase3/crdt/crdt_sets/discovery/gossip/identity/karma_gate/
karma_toolshare_filter/knowledge/lora/packet/peers/pipeline/sign/similarity/toolvalidate) +
`handlers_mesh*.go` (5) + agent `agentmgr/mesh.go` + `routerclient/mesh.go`.

## SEAM — nambah LAPIS FILTER baru TANPA buka frozen (2026-06-26, nutup gap audit)
**Akar:** L1-L9 dipanggil INLINE di `RunFilterPipeline` (FROZEN). Nambah lapis = kepaksa buka frozen.
**Fix (cabut-akar):** `internal/mesh/filter_ext.go` (**NON-frozen**) = registry `RegisterMeshFilter`
(switch-aware + fails-open/recover). `RunFilterPipeline` (frozen) di-hook 1 blok (`runExtraMeshFilters`
sebelum L9) → lapis tambahan jalan dalam pipeline yg sama.
**Nambah lapis baru (zero edit frozen):** file sibling `mesh/filter_<x>.go` →
`func init(){ RegisterMeshFilter(MeshFilter{Name,Switch:"FLOWORK_MESH_XXX",Run:func(db,pkt,content) FilterDecision}) }`
\+ switch di agent `internal/fwswitch/registry.go` → muncul GUI. Reject 1 lapis = pipeline stop.
**Test:** `TestRegisterMeshFilter`/`*SwitchAndReject`/`*FailsOpen` PASS. Re-freeze karma_toolshare_filter PASS.

## SWITCH
- **Per-lapis filter** = `MeshFilter.Switch` (switch-aware via registry.go) — tiap lapis baru bawa switch GUI.
- **Master mesh on/off**: BELUM ada (discovery/gossip start di `main.go` FROZEN, unconditional). Nambah
  `FLOWORK_MESH` butuh unfreeze main.go (entry-point) → sengaja TIDAK dikerjain (risiko, phase-1 dormant).
  Kalau owner mau, garap terpisah (unfreeze-protokol main.go).

## NON-FROZEN (seam)
`internal/mesh/filter_ext.go` (registry lapis filter), `mesh/filter_*.go` BARU (lapis tambahan),
`web/static/index.html` (GUI console), `routes.go`/`main.go` (route/boot — orchestration).

## VERIFIKASI 2026-06-26
QC live (:2402): identity (pubkey, 3 peer), stack (phase-1), peers 3, karma 2, knowledge 1, daemon
heartbeat, filter-pipeline jalan (9 lapis). Build router OK. Seam test PASS. TestKernelFreeze PASS.
Append frozen → "Operation not permitted".
