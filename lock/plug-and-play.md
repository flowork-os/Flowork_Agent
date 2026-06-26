# Plug-and-Play Audit — fitur eksternal copot-pasang

> Owner: Aola Sahidin (Mr.Dev) · floworkos.com. Prinsip: fitur EKSTERNAL wajib DATA/SEAM (tambah &
> share-mesh tanpa buka frozen). Arsitektur: lock/ARSITEKTUR.md. Cara bikin seam: flowork-secrets/CARAFREEZE.MD.

## Status (2026-06-27 — semua jalur EKSTERNAL plug-and-play)
| Fitur | Status | Nambah item BARU |
|---|---|---|
| Models / alias / custom | ✅ | DB (`/api/models/custom`,`/alias`, provider CfgModels) |
| Provider (koneksi) | ✅ | DB ProviderConnection (GUI) |
| Provider protokol/dialect | ✅ | sibling translator/{request,response} + `translator.Register` |
| Provider media (embed/img/tts/stt) | ✅ | sibling providers/<kat> + `Register` (STT: fixed via providers_register_ext.go) |
| Executors | ✅ | sibling internal/executors + `Register` |
| Combos | ✅ | DB (`/api/combos`) |
| Skills | ✅ | DB + registry pull/publish |
| Sensors/webhook | ✅ | ENV token + webhook generic |
| **Tunnel** | ✅ NEW | sibling `tunnel_<x>_ext.go` + `RegisterTunnelProvider` → /api/tunnel/providers |
| **Proxy deploy** | ✅ NEW | sibling `proxy_<x>_ext.go` + `RegisterProxyDeployTarget` → /api/proxy-pools/deploy/<x> |
| **CLI Tools** | ✅ NEW | sibling `cli_<x>.go` + `RegisterCLITool` → masuk All()/DetectAll() |
| **Cloaking** | ✅ NEW | profil via switch `FLOWORK_CLOAK_SUFFIX/VERSION/DECOYS` (GUI fwswitch, call-time) |
| MCP servers | ✅ | DB `mcpServer` + `mcpcatalog.Register` (3 default bawaan frozen = OK) |
| Presets | ✅ | versi dinamis = Combos (DB). `store.Presets` = starter bawaan frozen = OK |
| Auth (oidc/local/apikey) | 🔒 CORE | SENGAJA frozen (security inti, bukan plugin) |

## Prinsip yang dipakai (kenapa default bawaan boleh frozen)
Built-in DEFAULT (cloudflared/tailscale tunnel, cloudflare/deno/vercel proxy, 3 MCP default, preset
starter) = bagian ENGINE → boleh frozen. Yang WAJIB plug-and-play = **MENAMBAH yang baru** → semua
sudah via sibling+`Register*` ATAU DATA(DB) ATAU switch. Nol buka frozen utk nambah. Mekanisme
registry FROZEN (immutable, masuk root-hash integrity); extension = sibling/DATA non-frozen (deletable).

## Bukti tiap seam (live)
- Tunnel: `GET /api/tunnel/providers` → tailscale; `TestTunnelRegistry` PASS.
- Proxy: `GET /api/proxy-pools/deploy-targets` → cloudflare/deno/vercel; `TestProxyDeployRegistry` PASS.
- CLI: `/api/cli-tools` built-in tetap; `TestRegisterCLITool` PASS (dummy via Register muncul).
- Cloaking: `TestCloakProfileOverride` PASS (default + env override). Switch di GUI fwswitch.
- Semua: delete-test (hapus sibling non-frozen → build OK) + integrity tetap clean.

## 2 LEVEL plug-and-play (PENTING — owner 2026-06-27)
- **Backend seam (dev):** nambah lewat file sibling + `Register*()` / DATA → perlu rebuild. SUDAH (tunnel/proxy/cli/translator/executor/media).
- **GUI-CRUD (user):** tombol Add/Edit/Delete DI GUI (DB-backed) → user copot-pasang tanpa kode. Inilah "truth di GUI".

## Audit GUI-CRUD per tab (2026-06-27)
✅ FULL GUI-CRUD: providers, combos, models, proxy-pools, media(×5), pricing, skills, translator, brain.
⚠️ sebagian: mcp (add/del, no edit), api-keys (create/revoke), tags (create/del, no edit), oauth-imports (store/revoke, no edit).
❌ BELUM GUI-CRUD (owner sorot): **cli-tools** (read-only auto-detect), **tunnel** (cuma enable/disable cloudflare+tailscale — backend `/api/tunnel/providers` ADA, GUI belum), **mesh-console** (belum ada UI approve antrian + switch share/approve).

> Catatan: backend seam ≠ GUI-CRUD. Truth ada di GUI → target = tombol Add/Edit/Delete di layar.
> Frontend = `router/web/static/index.html` (NON-frozen). Roadmap kerjaan = `~/Documents/opus_roadmap.md` (luar repo).

## Sisa (opsional, low-value)
- Pindahin starter Presets / 3 MCP-default ke DB-seed: kosmetik (nambah sudah via DATA). Skip kecuali diminta.
