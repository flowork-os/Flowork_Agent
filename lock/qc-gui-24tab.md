# QC GUI 24-TAB — Verifikasi Logic-Frozen Jalan di GUI

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com

GUI Router `:2402` = 1 file `router/web/static/index.html` (go:embed). **GUI TIDAK di-freeze** (arahan
owner: yang di-freeze LOGIC, GUI wajib bisa evolusi). Tiap tab dipetakan ke `[data-tab]` → endpoint
`/api/*` → handler. Cara QC beneran (bukan curl doang): headless Chrome (`google-chrome` +
puppeteer-core) klik tiap tab, tangkap console/network error + screenshot, lalu uji aksi round-trip.

## Hasil
- **24 tab RENDER bersih**: data live, 0 error console, 0 HTTP 5xx.
- **Uji aksi round-trip NYATA**: Chat (reply LLM asli), Providers Test (`valid:true reachable`),
  API Keys generate+revoke (CRUD), Tags add+delete (CRUD), Settings PATCH (persist+revert),
  claude-login `/start` (PKCE + authorize-URL asli).
- **0 mock / 0 data palsu**. Penanda `NotImplemented` di handler = guard jujur (mis. "tailscale not
  installed", "provider not configured"), bukan stub nyamar. `bypass.StubText` = fitur Claude-CLI
  quota-saver yang di-switch dari Settings.
- **Test gate LOLOS**: `go build` · `go vet` · router `go test` · `TestKernelFreeze` (agent).

## Status freeze logic
SEMUA handler 24-tab sudah **FROZEN + `chattr +i`**. QC ini membuktikan logic-frozen itu beneran
berfungsi di GUI.

- **`router/handlers_claude_login.go` — DI-FREEZE 2026-06-26** (sebelumnya kelupaan; owner approve).
  Clean seam per-device Claude OAuth (sengaja tidak menyentuh oauth handler frozen). Proses:
  strip-komentar (cmtstrip, 716 token-kode IDENTIK via verifystrip = 0 perubahan kode) → header
  router-standar → sha256 `7284324b…f1b4` ke `KERNEL_FREEZE.md` → `chattr +i` (edit ditolak
  "Operation not permitted") → `TestKernelFreeze` PASS → router build OK → `/start` live.
  Catatan: `/complete` butuh login OAuth interaktif owner (belum diuji end-to-end); kalau perlu
  ubah, unfreeze (`chattr -i`) → edit → re-hash → re-freeze.

Aksi ber-efek-eksternal yang TIDAK dipicu saat QC (perlu kondisi nyata, bukan bug): Tunnel enable
(buka ke internet), MITM start (butuh admin + intercept traffic), CLI Tools Configure (nulis file
config CLI asli), Proxy deploy (deploy ke Cloudflare/Vercel/Deno + butuh kredensial). Semua render +
endpoint status-nya live; eksekusi penuh di-skip karena efek samping nyata.
