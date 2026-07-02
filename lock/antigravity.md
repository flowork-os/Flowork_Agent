# Antigravity provider (plug-and-play) — 2026-07-02

## Prinsip (owner: Flowork ABADI)
Fitur gantung pihak-ke-3 = COLOKAN. Google matiin Antigravity → hapus sibling,
core router UTUH. Semua lewat seam; Claude TIDAK disentuh.

## Alur
- **Flow A (capture):** app Antigravity di-MITM ke router (`~/.flow_router/antigravity-mitm.json`,
  interceptHosts cloudcode-pa). Handler MITM "antigravity" (override sibling) NANGKEP
  Bearer + header client asli → persist (OAuth token + provider auto + header KV).
- **Flow B (pakai):** mr-flow manggil `gemini-3` → executor antigravity → Google
  cloudcode-pa pakai token+header hasil capture (lolos validasi client Google).

## File
| File | Status | Peran |
|---|---|---|
| `router/internal/mitm/handlers/antigravity_capture.go` | non-frozen sibling | override handler "antigravity", capture Bearer+header, delegate reroute |
| `router/internal/executors/antigravity.go` | FROZEN (re-hash) | +seam `AntigravityHeaderHook` (default nil = header lama) |
| `router/antigravity_ext.go` | non-frozen | wiring: persist creds + provider auto (`antigravity-auto`, models gemini-3/…) + header hook + token loader |
| `router/internal/creds/imports.go` | FROZEN (re-hash) | +papan `RegisterDetector` (OAuth import plug-and-play) |
| `router/handlers_oauth.go` | FROZEN (re-hash) | +papan `RegisterTokenLoader` |
| `router/internal/creds/imports_antigravity.go` | non-frozen sibling | detektor Antigravity + gemini-cli di dropdown OAuth Imports |

## Switch
- `FLOWORK_MODEL_REMAP` (lihat model-deprecat) — beda fitur.
- `FLOWORK_ANTIGRAVITY_CAPTURE` (default ON) — matiin capture → executor pakai header default.
- `FLOWORK_ANTIGRAVITY_EMPTY_OK` (default OFF) — ON = teruskan response kosong apa
  adanya (perilaku lama). OFF = response tanpa teks → executor return error → fallback.

## AKAR "AI ngebalikin jawaban kosong" (fix 2026-07-02)
gemini-3.x-pro (thinking) di jalur tool-heavy (mr-flow) kadang balik candidate TANPA
teks: part cuma `{thoughtSignature}` + `finishReason=MALFORMED_FUNCTION_CALL` — nyoba
tool-call tapi malformed (executor GA forward `functionDeclarations`, model improvisasi).
Dulu diteruskan sbg 200 → user liat "jawaban kosong". Fix di `antigravity.go` NonStream:
teks kosong = GAGAL → return error (502) → dispatcher FALLBACK ke provider berikut (mis.
gemini-3.5-flash-low / Claude) yg jawabnya bener. `antigravityRespToOpenAI` skrg balikin
`(json, text, rawFinish)` biar caller bisa deteksi kosong. Verified live: mr-flow bahasa
manusia "apa rencana selanjutnya?" → jawaban penuh (bukan kosong). Switch balik: EMPTY_OK=1.

## Model
Provider auto advertise: `gemini-3, gemini-3-pro, gemini-3-flash, gemini-2.5-pro,
gemini-2.5-flash` (SENGAJA bukan glob `gemini-*` biar ga hijack vertex/gemini_cli).
mr-flow di-pin `gemini-3` (state.db `router_model`).

## ⚠️ Butuh 1x aksi owner (LIVE)
Token = AUTO-CAPTURE lewat MITM → **jalanin app Antigravity sekali** (biar request
lewat router) → provider `antigravity-auto` otomatis aktif + token ke-simpen →
mr-flow gemini-3 jalan. SEBELUM itu: provider `[off]`, gemini-3 fallback ke chain
(bisa kena Claude). Verifikasi: `sqlite3 ~/.flow_router/db/data.sqlite "SELECT id,isActive
FROM providerConnections WHERE id='antigravity-auto';"` → isActive=1.

## QC 2026-07-02
build/vet/test (creds+executors+main)/TestKernelFreeze/delete-test hijau. Unit:
header-inject (captured menang + Bearer fresh + provider auto), capture-OFF→nil.
Test ISOLASI DB (TestMain FLOW_ROUTER_DATA temp) — DB router asli ga kesampah.

## UPDATE 2026-07-02 — LIVE via OAuth (MITM mentok DoH, ganti OAuth login)
Cara final (bukan MITM — app pakai DNS-over-HTTPS, 9router #1356):
- `router/antigravity_oauth_ext.go` (non-frozen): OAuth 2.0 PKCE login. Endpoint
  `/api/oauth/antigravity/start` (auth URL + listener :51121) · `/status`. Token +
  refresh + project (loadCodeAssist) di DB. Auto-refresh. GUI tombol "Login with
  Google" di OAuth Imports (di bawah Claude).
- ANTI-HARDCODE: client_secret di-extract runtime dari binary `language_server`;
  client_id switch `FLOWORK_ANTIGRAVITY_CLIENT_ID` (default resmi); model switch
  `FLOWORK_ANTIGRAVITY_MODELS`.
- Executor (FROZEN, re-hash): body +userAgent +requestId (wajib, tanpa=400);
  NonStream translate respons Gemini nested {response:{candidates}} → OpenAI
  {choices}. Provider Data.format='antigravity' (WAJIB, tanpa=proxy generik=404).
- **Model VALID (verified live): `gemini-3.1-pro-low`, `gemini-3.5-flash-low`**
  (-high/-preview → 400/404). mr-flow di-pin `gemini-3.1-pro-low`.
- VERIFIED: `dispatch model=gemini-3.1-pro-low → provider=Antigravity tokens=4309+`.
  Claude TIDAK disentuh (tetap OK). Provider `antigravity-auto` (1, konsolidasi).
- Setup ulang (device baru): login app Antigravity → GUI "Login with Google" → done.
