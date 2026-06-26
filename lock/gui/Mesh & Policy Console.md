# Mesh & Policy Console

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com
> Dok tab GUI Flowork Router (:2402). Standar freeze: lock/frozen-core.md.

## Fungsi
Konsol terpadu untuk **jaringan kedaulatan (mesh)** antar-node Flowork + **engine kebijakan/biaya**. Menampilkan identitas node (pubkey Ed25519), peer, paket ber-tanda-tangan, knowledge inbox (anti-poisoning), tool manifest, karma peer, uji pipeline filter, provider-chain, registry LocalAI, kalkulator harga, dan budget/violation kebijakan.

## Endpoint (router/routes.go)
- **Identity & Peers**: `GET /api/mesh/identity`, `/peers`, `POST /api/mesh/discover`, `/peer`, `/peer/block` → handlers_mesh.go.
- **Paket transport**: `POST /api/mesh/packet`, `/packet/send`, `GET /api/mesh/packets` → handlers_mesh_transport.go (verify Ed25519 + rate-limit + dedup + HopMax).
- **Advanced (loopback-guard utk mutasi)**: `/api/mesh/crdt`, `/knowledge`, `/tool-manifests`, `/karma`, `/karma/decay`, `/filter/test`, `/lora-deltas`, `/l3`, `/daemon/status` → handlers_mesh_advanced.go.
- **Overview**: `GET /api/mesh/stack/overview` → handlers_mesh_stack.go.
- **Policy/Pricing** (handlers_llm_policy.go): `/api/provider/chains`, `/provider/calls`, `/api/localai/models`, `/localai/runtime`, `/api/pricing/rules`, `/pricing/calc`, `/pricing/log_call`, `/api/policy/budgets`, `/policy/violations`, `POST /api/policy/tick`.

## Logic / Alur
- **Provenance**: tiap paket mesh ditandatangani Ed25519 (`internal/mesh/sign.go`); `ParsePacketJSON` verify sebelum diproses.
- **Anti-flood/Sybil**: rate-limit per-source IP (sliding window 10s, maks 60 paket) — handlers_mesh_ratelimit.go.
- **Anti-poisoning**: knowledge masuk lewat pipeline filter 9-lapis (`internal/mesh/pipeline.go`) + near-dup + karma; mutasi CRDT/knowledge **loopback-only** (`isLoopbackHostPort`) → node remote tak bisa racuni state.
- **Policy engine**: `internal/policy/evaluator.go` di-wire di `main.go` (`policyEngineRef`), tick interval 5m; `POST /api/policy/tick` = sweep manual (live: `{evaluated:1, fired:0, ok:true}`). Aksi: warn (log) / block (429 di `/v1`).
- **Pricing**: tiered rules per model → `pricing/calc` hitung biaya; `provider/calls` log pemakaian.

## File yang dilewati
- `router/routes.go`; `router/handlers_mesh.go`, `_transport.go`, `_advanced.go`, `_stack.go`, `_ratelimit.go`; `router/handlers_llm_policy.go`.
- `router/internal/mesh/` — sign.go, packet.go, peers.go, identity.go, crdt.go, knowledge.go, pipeline.go, karma_gate.go, toolvalidate.go, lora.go.
- `router/internal/policy/evaluator.go`.
- `router/internal/store/` — mesh_migrations.go, llm_pricing_policy_migrations.go.
- `router/web/static/index.html` — `data-tab="mesh-console"` (~baris 1278-1425): `loadMeshConsole` + listener per-aksi.

## Teknologi
- Transport HTTP/REST (Phase 1-2); ed25519 detached signature.
- Rate-limit sliding-window in-memory; loopback-guard untuk mutasi sensitif.
- CRDT counter-wins; pipeline filter 9-lapis; policy engine interval-tick (5m).
- SQLite (mesh_* & policy_* migrations).

## SWITCH / seam evolusi
- Provider-chain, pricing-rule, policy-budget, LocalAI-model = **DATA** (baris DB lewat GUI) → tak hardcode.
- Interval/perilaku policy via fwswitch; endpoint mesh baru = `*_ext.go` + `RegisterExtraRoute` (routes_ext.go).

## Catatan jujur (bukan bug / data palsu)
- `POST /api/mesh/discover` = **guard jujur** balik `{phase:1, "discovery stub — phase 2 mDNS"}` (single-owner; mDNS multicast = Phase 2). Bukan data palsu.
- mTLS/HTTP-2 streaming/relay = roadmap Phase 3 (komentar, belum perlu utk single-owner).
- Nilai default tombol uji di GUI (mis. `origin_pubkey: 'abcd1234'`, filter `sha256:abcd`) = placeholder form diagnostik di `index.html` (NON-frozen, bisa diperbaiki kapan saja agar ambil identitas asli).

## Status freeze (QC 2026-06-26)
- Live GUI/API: identity, peers, packets, karma, filter/test, policy/tick, pricing/calc JALAN.
- FROZEN: semua handlers_mesh*.go + handlers_llm_policy.go + internal/mesh/* + internal/policy/*.
- NON-FROZEN (sengaja): `index.html` (GUI), `internal/fwswitch/registry.go` (switch), `routes_ext.go` (seam).
