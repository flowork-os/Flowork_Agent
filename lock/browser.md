# BROWSER-CONTROL — Tool Browser Native (Go, no node): Cara Kerja, Tool, Cookie & Cabang

> Dokumen referensi (white-label). Menjelaskan SEMUA soal browser-control: cara kerja,
> daftar tool, cookie-inject, lifecycle/keamanan, env, titik-extension (cabang), freeze.
> Owner: Aola Sahidin (Mr.Dev). Repo: https://github.com/flowork-os/Flowork-OS.
> Update terakhir: 2026-06-23.
> ⚠️ File ini KE-TRACK repo → NOL data personal/cookie owner (mekanisme generic doang).

---

## ⛔ WAJIB BACA DULU (buat AI/dev yang mau ngedit)

File inti `internal/tools/builtins/browser_desktop.go` **DI-FREEZE** (chattr +i + hash
`KERNEL_FREEZE.md`). **JANGAN buka / edit file frozen.** Kalau mau nambah filtur:

1. **TOOL browser baru** (mis. `browser_scroll`) → bikin **FILE BARU** `browser_<nama>.go`
   (build-tag sama) dgn `func init(){ tools.Register(&browserXxxTool{}) }`. Go gabung semua
   init() sepaket → tool ke-daftar TANPA edit file frozen.
2. **Tuning launch/lifecycle** (headless, flag chromium, idle timeout) → **env switch** yang
   dibaca **`browser_desktop_ext.go`** (NON-frozen). Lihat §6.

Filosofi owner: **"file frozen ngak akan pernah dibuka lagi"**. Tiap titik switch DI DALAM
file frozen udah dikasih komentar penunjuk ke cabang (`browser_desktop_ext.go`) — kalau lo
(AI berikut) ngeliat itu, JANGAN edit di situ, pergi ke cabang. Mentok? minta izin owner.

---

## 0. APA INI + KENAPA

Browser-control = **ngendaliin Chromium ASLI dari Go** (lewat CDP/DevTools Protocol, lib
`go-rod`) — **TANPA node/puppeteer**. Alasan (roadmap "Opsi B", cabut-gigi bukan tambal):
- `webfetch` sering ke-blokir Cloudflare/JS-heavy → browser asli LOLOS (render beneran).
- Node = dependency berat + ga portable di OS-image. Go-native = 1 binary, cross-compile.
- Bisa **inject cookie** → otomasi situs login (mis. posting Facebook) tanpa ketik password.

Build-tag: `(linux || darwin || windows) && !android` → **desktop-only**. Di android tool ini
ga ke-compile (ga ada chromium) — primitive `browser` juga ga ke-daftar (aman).

---

## 1. ARSITEKTUR

```
agent (WASM)  ──tool browser_*──▶  HOST (browser_desktop.go)  ──go-rod/CDP──▶  Chromium
                                         │ singleton: 1 browser + 1 page (brInst/brPage)
                                         │ lazy launch (getPage), profil persisten
                                         ▼
                                   ~/.flowork/browser-profile  (cookie nempel antar-sesi)
```

- **Singleton**: 1 instance browser + 1 page, di-protect `brMu` (mutex). Lazy: baru launch
  pas tool pertama dipanggil (`getPage`).
- **Profil persisten**: `~/.flowork/browser-profile` (fallback `$TMPDIR/flowork-browser-profile`)
  → cookie/login nempel antar-sesi.
- **2 mode**:
  - *launch-mode* (default): host nge-launch chromium headless sendiri.
  - *connect-mode*: env `FLOWORK_BROWSER_URL` di-set → nempel ke Chrome yang udah jalan
    (sesi+cookie login owner kepakai). Buat kasus "contek profil Chrome yang lagi kebuka".
- **Capability**: `browser:control`. Di-whitelist lewat `loader.RegisterPrimitive("browser")`
  (hook NON-frozen di manifest.go) → manifest agent boleh deklarasi `browser:control` tanpa
  bongkar loader/manifest.go yang frozen.

---

## 2. DAFTAR TOOL (9) — semua cap `browser:control`

| Tool | Fungsi |
|---|---|
| `browser_navigate` | buka URL (render JS, lolos CF). Balik {url,title}. Abis ini `browser_snapshot` |
| `browser_snapshot` | "lihat" halaman: elemen interaktif (a/button/input/...) dikasih `data-fw-uid` + label → aksi by-uid |
| `browser_click` | klik elemen by-uid |
| `browser_type` | ketik teks ke elemen by-uid (focus + CDP InsertText) |
| `browser_upload` | upload file ke `<input type=file>` by-uid |
| `browser_screenshot` | screenshot halaman (PNG) |
| `browser_set_cookies` | **inject cookie** (sesi login tanpa password) |
| `browser_eval` | jalanin JS arbitrary di page (escape hatch) |
| `browser_close` | **tutup browser** — WAJIB tiap tugas kelar |

Pola pemakaian: `navigate` → `snapshot` (lihat uid) → `click`/`type`/`upload` (by-uid) →
`screenshot` (verifikasi) → `close`.

---

## 3. COOKIE-INJECT (sesi login tanpa password)

`browser_set_cookies` param `cookies` = JSON array:
```json
[{"name":"c_user","value":"...","domain":".facebook.com","path":"/"}]
```
Inject via CDP (`proto.NetworkSetCookies`) → `navigate` ke situsnya → udah login.

⚠️ **KEAMANAN (sacred):** cookie = SECRET sesi. JANGAN pernah di-print ke chat/log, JANGAN
commit. Cookie owner di-extract dari profil Chrome (gnome-keyring v11: `Chrome Safe Storage`
→ PBKDF2(pw,"saltysalt",1,16) → AES-128-CBC; strip 32-byte SHA256 prefix kalau non-printable
[Chrome v124+]) — itu proses TERPISAH (script unduh), BUKAN di file ini. File temp cookie =
shred abis pakai.

---

## 4. LIFECYCLE & KEAMANAN (anti zombie chromium)

3 lapis biar chromium ga numpuk:
1. **`browser_close` eksplisit** — skill WAJIB manggil tiap tugas kelar (closeBrowser idempoten).
2. **Idle-reaper IN-AGENT** — `startBrowserReaper` (sekali, `sync.Once`): browser nganggur
   `> browserIdleTimeout()` (default **30 menit**) → auto-close. Backstop kalau lupa close.
3. **Orphan-reaper DOCKTOR** — `os/selfheal/watchdog.sh`: chromium YATIM (match
   `user-data-dir=*flowork-browser-profile*` + PPID=1 = agent crash mid-tugas) di-kill.
   (Cuma profil Flowork — BUKAN Chrome owner; beda profil.)

Tiap tool di-wrap `pageTimeout` (mis. navigate 45s) biar ga gantung selamanya.

---

## 5. ENV CONFIG (semua override TANPA unfreeze)

| Env | Default | Fungsi | Dibaca |
|---|---|---|---|
| `FLOWORK_CHROME_BIN` | auto-detect | path binary chromium | browser_desktop.go (frozen) |
| `FLOWORK_BROWSER_URL` | (kosong) | connect-mode: nempel Chrome jalan | browser_desktop.go (frozen) |
| `FLOWORK_BROWSER_HEADLESS` | `1` (headless) | `0`/`off` = headful (debug, lihat jendela) | **browser_desktop_ext.go** |
| `FLOWORK_BROWSER_FLAGS` | (kosong) | flag chromium tambahan, dipisah koma | **browser_desktop_ext.go** |
| `FLOWORK_BROWSER_IDLE_MIN` | `30` | menit idle sebelum auto-close | **browser_desktop_ext.go** |

---

## 6. CABANG ABADI (extension point) — `browser_desktop_ext.go` (NON-frozen)

Realisasi "file frozen ga pernah dibuka lagi". File frozen cuma MANGGIL fungsi ext + tiap
titik dikasih komentar penunjuk.

| Yang mau diubah | Caranya (TANPA buka frozen) |
|---|---|
| Tambah TOOL browser baru | FILE BARU `browser_<nama>.go` + init() sendiri (Go gabung init sepaket) |
| Headless on/off | env `FLOWORK_BROWSER_HEADLESS` → `browserHeadless()` |
| Flag chromium tambahan | env `FLOWORK_BROWSER_FLAGS` → `applyExtraBrowserFlags()` |
| Idle timeout | env `FLOWORK_BROWSER_IDLE_MIN` → `browserIdleTimeout()` |
| Hook launch/lifecycle lain | tambah fungsi di `browser_desktop_ext.go`, panggil dari titik berkomentar |

Titik switch di file frozen (udah ada komentar `// CABANG abadi ...`): `init()` (tool baru),
`getPage()` launch (headless+flags), `startBrowserReaper` (idle timeout).

---

## 7. GOTCHA PENTING (jangan di-"perbaiki" balik!)

- **Facebook (& SPA berat) sering ganti `<div>`** → tutorial/aksi PATOKANNYA pola (snapshot
  by-uid), BUKAN selector hardcode.
- **`browser_click` pakai `el.Eval("() => this.click()")`**, BUKAN `el.Click` go-rod —
  el.Click nge-wait "stable/interactable" → **HANG** di FB ("context deadline").
- **`browser_type` pakai focus + CDP `p.InsertText`**, BUKAN `el.Input` — el.Input bisa HANG
  di SPA berat; InsertText nge-dispatch input event yg React/Lexical nangkep.
- Kalau AI berikut ngeliat ini & ngira "bug, mestinya el.Click" → **STOP**, ini SENGAJA
  (fix empiris FB). Jangan dibalik.

---

## 8. PETA FILE & FREEZE

| File | Peran | Freeze |
|---|---|---|
| `internal/tools/builtins/browser_desktop.go` | CORE: 9 tool + getPage + reaper + lifecycle | **FREEZE** |
| `internal/tools/builtins/browser_desktop_ext.go` | CABANG: env switch (headless/flags/idle) | NON-frozen |
| `internal/kernel/loader/manifest.go` | whitelist primitive (browser via RegisterPrimitive hook) | FROZEN (hook NON-frozen) |
| `os/selfheal/watchdog.sh` | orphan chromium reaper | NON-frozen (lapis ops) |
| `browser_<nama>.go` (masa depan) | tool browser baru | NON-frozen (file baru) |

---

## 9. PANTANGAN

- ❌ Jangan print/commit cookie atau file profil (cookie = secret sesi).
- ❌ Jangan balikin `el.Click`/`el.Input` (sengaja diganti JS-click + InsertText, fix FB-hang).
- ❌ Jangan hardcode selector situs (FB ganti div mulu — pakai snapshot by-uid).
- ❌ Jangan lupa `browser_close` tiap tugas kelar (ada 3 backstop, tapi tetep boros kalau lupa).
- ❌ Jangan buka file FROZEN buat filtur baru — tool baru = file baru; tuning = env (§6).
