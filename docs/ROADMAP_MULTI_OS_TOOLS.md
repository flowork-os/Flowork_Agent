# ROADMAP — Multi-OS Plug-and-Play Tools

> **Status:** PLAN (belum dieksekusi). Ditulis 2026-06-23 oleh Claude (Opus 4.8) atas
> arahan owner Aola. Dokumen ini SENGAJA self-contained — kalau AI lain (konteks beda /
> habis) ngelanjutin, baca ini dari atas, ga perlu konteks chat sebelumnya.
>
> ⚖️ Patuhi `lock/brain.md` (konstitusi arsitektur) + Rule Emas Flowork sebelum eksekusi.

---

## 0. TL;DR (buat AI yang baru masuk)

Flowork mau jadi kayak **WordPress**: core polos, fitur = **plugin**. Di Flowork "plugin" =
**tools**. Pendorong utamanya **MULTI-OS**: tool itu beda implementasi per platform —
"buka aplikasi", "kontrol browser", "matiin sistem", "shell" beda total di
Linux / Android / Windows / macOS. Jadi tool **ga boleh** hardcode satu OS; tiap binary OS
harus bawa set tool-nya sendiri.

**Keputusan arsitektur (FINAL, sudah didiskusikan + diverifikasi):**

1. **Tool universal** (brain, memory, file, web, dst, ~110 tool) → tetap shared, 1 file each.
2. **Tool platform-divergent** (~10-15: shell, app_open, system_power, browser, clipboard,
   notif) → **di-SPLIT per-OS pakai Go build-tag** (`_linux.go` / `_android.go` /
   `_windows.go` / `_darwin.go`). **Inilah "1 tool, file-per-platform" yang BENER** — bukan
   180 `.fwpack` palsu.
3. **Tool add-on** → **WASM `.fwpack`** (portable, jalan di semua OS via WASM runtime di
   binary). Ini lapis plugin "WordPress"-nya. GUI upload-nya sudah ada (tab **Tools**).
4. **Akses per-agent** → subscription (sudah ada, GUI tab Tools di tiap Agent).

**KENAPA build-tag, bukan plugin native runtime:** Android **melarang** load native code
runtime (security) + mobile pakai binary Go yang SAMA (cross-compile `GOOS=android`). Jadi
tool privileged WAJIB compiled-in per-platform. WASM `.fwpack` tetap jalan buat add-on.

**KENAPA bukan jadiin 180 tool → 180 `.fwpack`:** 125 dari 183 tool itu **host-privileged**
(exec/fs/secret/state/rpc). `.fwpack` = WASM **sandbox** yang sengaja nolak akses itu → bakal
jadi 125 file palsu/mati. Plus tool privileged = jantung yang HARUS beku & aman (kalau `bash`
bisa di-swap, AI jahat tinggal upload `bash` palsu → tamat). Frozen-kernel + sandboxed-plugin
= **pertahanan**, bukan keterbatasan.

---

## 1. State terverifikasi per 2026-06-23 (fakta + lokasi file)

### Inventaris tool (live, dari `/api/agents/tools/catalog?id=mr-flow`)
- **183 tool** total ter-expose ke mr-flow: **137 builtin** (compiled-in) + **46 app** (`app_flowalpha_*`).
- Capability: **125 host-privileged** (exec/fs/secret/state/rpc) · 9 net/mcp · **4 pure-compute** (echo, now, system_health, StructuredOutput).

### Infra plug-and-play yang SUDAH ADA (terverifikasi di kode)
- `agent/internal/tools/types.go` — interface `Tool` (Name/Schema/Capability/Run).
- `agent/internal/tools/registry.go` — registry statik (builtin, panic-on-dup).
- `agent/internal/tools/dynamic.go` — `RegisterDynamic()` / `Unregister()` / `DynamicNames()` (runtime add/remove tanpa rebuild).
- `agent/tool_install.go` — `POST /api/tools/install` (.fwpack multipart) · `/uninstall` · `/installed` (filter ke tool-pack asli via marker `tool.json`). Route di `feature_dev.go:30-32`.
- `agent/internal/mcphub/mcphub.go` — MCP bridge: tiap tool MCP → `RegisterDynamic` jadi `mcp_<id>_<tool>`.
- `agent/internal/agentmgr/tool_subscriptions.go` + `tool_specs.go` — akses per-agent (subscribe/unsubscribe, DB `tool_subscriptions`, cap expose 52).
- GUI: `agent/web/tabs/tools.js` (tab **Tools**, upload/list/uninstall — dibuat sesi ini) + `agent/web/tabs/agents_tool_catalog.js` (subscribe per-agent).
- Auto-install: watcher `~/.flowork/dropbox` (drop `.fwpack` → auto-install).

### Multi-OS HARI INI (yang mau diperbaiki)
- Ditangani via **`runtime.GOOS` if-branch DI DALAM satu file** (bukan build-tag).
- File pakai `runtime.GOOS`: `internal/tools/builtins/{app_open,shell,system_power,claude_tools,shell_guard,v4_extras}.go`.
- Cuma **1** file build-tag: `internal/tools/builtins/shell_rlimit_linux.go`.
- **Masalah:** kode Windows (`cmd.exe`) ikut ke-bundle di binary Android (sampah) · ga bisa tool eksklusif-OS · nambah OS = ngutak-atik file shared (lawan prinsip nano-modular/freeze).

### Mobile (FLowork_Mobile/)
- Android native (Kotlin/Gradle), repo TERPISAH dari FLowork_os, **reuse 100% Go core**.
- `scripts/build-core.sh`: `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -buildmode=pie`
  pada `../FLowork_os/agent` + `router` → `app/src/main/jniLibs/arm64-v8a/lib*.so`.
- APK = Foreground Service exec binary → WebView ke `127.0.0.1:1987` (panel sama desktop).
- **Tanpa NDK, CGO off** → ga bisa dlopen native plugin → tool privileged WAJIB compiled-in.

### Status freeze (jawaban pertanyaan owner)
- **Semua file tool platform-divergent EDITABLE** (terverifikasi `lsattr`): shell.go, app_open.go,
  system_power.go, claude_tools.go, shell_guard.go, v4_extras.go, builtins.go.
- Yang FROZEN = brain-core (cognition/recall/constitution/mesh/dst, 131 hash di
  `KERNEL_FREEZE.md`), termasuk `builtins_brain.go` (tool brain — UNIVERSAL, ga di-split).
- **→ Refactor multi-OS tool TIDAK menyentuh file frozen.** (Detail §5.)

---

## 2. Model arsitektur (3 lapis)

```
┌──────────────────────────────────────────────────────────────┐
│ L1 TOOL UNIVERSAL (~110) — brain/memory/file/web/audit/...    │  shared, 1 file each
│    jalan di semua OS. (brain tools = FROZEN, universal)       │  (sebagian frozen)
├──────────────────────────────────────────────────────────────┤
│ L2 TOOL PLATFORM-DIVERGENT (~10-15) — shell/app_open/power/   │  SPLIT build-tag per-OS
│    browser/clipboard/notif → file-per-platform                │  (editable growth)
├──────────────────────────────────────────────────────────────┤
│ L3 TOOL ADD-ON — WASM .fwpack (upload/uninstall)             │  plugin layer (portable)
└──────────────────────────────────────────────────────────────┘
   AKSES: subscription per-agent (sudah ada). Katalog auto nampilin
   set yang ke-compile buat OS itu.
```

---

## 3. Pola build-tag split (teknik inti L2)

Untuk tiap tool platform-divergent `X`:

```
internal/tools/builtins/
  X.go              ← struct tool + Name/Schema/Capability + registrasi; panggil xRun(ctx,args)
  X_linux.go        //go:build linux      → impl Linux  (mis. xdg-open, systemctl, /bin/sh)
  X_android.go      //go:build android    → impl Android (mis. `am start` Intent, toybox sh)
  X_windows.go      //go:build windows    → impl Windows (mis. start, powershell, shutdown)
  X_darwin.go       //go:build darwin     → impl macOS  (mis. open, osascript)
  X_other.go        //go:build !linux && !android && !windows && !darwin  → stub "unsupported"
```

Aturan:
- File shared (`X.go`) **ga boleh** `runtime.GOOS` lagi — semua cabang OS pindah ke file `_<os>.go`.
- Tiap symbol platform (`xRun`) WAJIB punya impl di SETIAP target OS **atau** ada `_other.go`
  fallback — kalau ga, build OS itu PATAH (build constraint Go).
- **Tool eksklusif-OS**: cukup taruh registrasi + impl di `_<os>.go` doang (mis.
  `android_notif_android.go` `//go:build android`) → cuma ke-compile + ke-register di Android.
  Di OS lain ga ada → katalog OS lain ga nampilin (otomatis benar).
- Registrasi: kalau tool ga ada di suatu OS, jangan daftarin di `builtins.go` (yang shared);
  daftarin di file `_<os>.go` lewat `init()` atau lewat fungsi `registerPlatform<X>()` yang
  cuma ada di OS itu. (Pilih satu pola, konsisten.)

---

## 4. Work-list tool yang di-split (refine di Fase 0)

Kandidat awal (yang sekarang pakai `runtime.GOOS`):
| Tool | Linux | Android | Windows | macOS |
|---|---|---|---|---|
| `shell`/`bash` | `/bin/sh` | toybox/`sh` | `cmd`/`powershell` | `/bin/sh` |
| `app_open` | `xdg-open` | `am start` Intent | `start` | `open` |
| `system_power` | `systemctl`/`loginctl` | (terbatas/API) | `shutdown` | `osascript`/`pmset` |
| `shell_guard` | rlimit Linux | cgroup/none | job object | ulimit |
| `claude_tools` | (cek GOOS-nya) | — | — | — |
| `v4_extras` | (cek GOOS-nya) | — | — | — |

Tool **baru eksklusif** yang mungkin per-OS (contoh, bukan wajib):
- Android: `android_notif`, `android_sms`, `android_share`, `android_clipboard`, browser via Custom Tabs.
- Desktop: `clipboard`, `screenshot`, `window_control`.

> Fase 0 WAJIB: audit ulang `grep -rn runtime.GOOS internal/tools/` + cek tiap tool, hasilkan
> work-list final sebelum nyentuh kode.

---

## 5. Interaksi FREEZE (jawaban eksplisit pertanyaan owner)

**Refactor ini TIDAK menyentuh file frozen.** Alasan:
- Semua tool platform-divergent (shell/app_open/system_power/claude_tools/shell_guard/v4_extras)
  + `builtins.go` = **EDITABLE** (terverifikasi).
- File `_<os>.go` BARU = growth-layer, **editable**, **TIDAK** masuk `KERNEL_FREEZE.md`
  (tool platform memang harus boleh evolve per-OS; bukan jantung).
- Tool brain (`builtins_brain.go`, FROZEN) = universal, **ga di-split**.

**Kalau (jarang) split butuh nyentuh file frozen:** pakai pola baku
`sudo chattr -i <file>` → edit → re-hash di `KERNEL_FREEZE.md` → `sudo chattr +i` →
`cd agent && GOWORK=off go test -run TestKernelFreeze .` (harus PASS di jumlah hash terbaru).
Update `lock/brain.md` STATUS + catatan. (Lihat lock/brain.md §13 + flowork-freeze-protocol.)

---

## 6. Langkah eksekusi (per fase, rollback-safe)

- **Fase 0 — Audit.** `grep -rn runtime.GOOS internal/tools/` → work-list final + matrix OS.
  Tentukan pola registrasi (init per-file vs registerPlatform). Output: daftar pasti.
- **Fase 1 — Template 1 tool.** Split `app_open` jadi `app_open.go` + `app_open_{linux,android,other}.go`.
  Build VERIFY 2 target:
  - `cd agent && GOWORK=off go build ./...` (host Linux)
  - `cd agent && GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -buildmode=pie -o /tmp/and-agent .`
  Jadiin template + commit.
- **Fase 2 — Split sisa divergent tool**, SATU tool per commit, tiap commit build-verify 2 target
  (linux + android). Smoke test runtime di Linux (login + tool catalog).
- **Fase 3 — Tool eksklusif-OS** (Android notif/sms/dst sebagai `_android.go`).
- **Fase 4 — Katalog/GUI per-platform.** Pastikan `/api/agents/tools/catalog` + subscription
  benar per-platform. Opsional: tambah field `platforms` di schema/manifest buat display.
- **Fase 5 — Policy "tanpa tools default"** (opsional, visi owner): agent baru = 0 tool subscribe,
  owner isi sendiri. Ubah default-exposure (`coreExposedTools` di `tool_specs.go` skrg 15 auto).
- **Fase 6 — Contoh `.fwpack`.** Bikin 2-3 tool WASM pure-compute (kalkulator/regex/json) +
  dokumen cara bikin tool-pack (jadi template komunitas, mirror skill registry).

---

## 7. Kriteria sukses / verifikasi tiap langkah

- ✅ `GOOS=linux go build ./...` PASS.
- ✅ `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -buildmode=pie` PASS (cross-compile mobile).
- ✅ Binary Android **tidak** bawa kode Windows/Linux-only buat tool divergent (cek via build-tag, bukan GOOS-branch).
- ✅ Katalog tiap platform nampilin set tool yang benar.
- ✅ `TestKernelFreeze` tetap PASS di jumlah hash terkini (= 0 file frozen tersentuh).
- ✅ Subscription per-agent tetap jalan; mr-flow recall + notif tetap normal.

---

## 8. Referensi file (buat AI pelaksana)

| Area | Path |
|---|---|
| Interface tool | `agent/internal/tools/types.go` |
| Registry statik | `agent/internal/tools/registry.go` |
| Registry dinamis (plugin) | `agent/internal/tools/dynamic.go` |
| Daftar builtin (Init) | `agent/internal/tools/builtins/builtins.go` (EDITABLE) |
| Tool brain (FROZEN, universal) | `agent/internal/tools/builtins/builtins_brain.go` |
| Tool divergent (target split) | `agent/internal/tools/builtins/{app_open,shell,system_power,shell_guard,claude_tools,v4_extras}.go` |
| Install .fwpack | `agent/tool_install.go` + route `agent/feature_dev.go` |
| Akses per-agent | `agent/internal/agentmgr/{tool_subscriptions,tool_specs}.go` |
| GUI tab Tools | `agent/web/tabs/tools.js` · per-agent: `agent/web/tabs/agents_tool_catalog.js` |
| Build host | `agent/start.sh` · packager .fwpack: `agent/scripts/build-agent.sh` |
| Cross-compile mobile | `FLowork_Mobile/scripts/build-core.sh` |
| Freeze | `lock/brain.md` · `KERNEL_FREEZE.md` (symlink→flowork-secrets) · `TestKernelFreeze` |

---

## 9. Gotcha / kendala (JANGAN kepleset)

- **Build constraint Go:** tiap symbol platform butuh impl di SETIAP target OS atau `_other.go`
  fallback, kalau ga build OS itu patah. Selalu build-verify linux + android tiap commit.
- **Mobile:** `CGO_ENABLED=0`, no NDK, no dlopen → privileged tool WAJIB build-tag, BUKAN runtime plugin.
- **Toolchain (dari pengalaman, NON-obvious):** kalau build via script yang ke-trigger dari root
  monorepo, set `GOWORK=off` (ada `go.work` minta go≥1.25) + `GOTOOLCHAIN=local`
  (cegah go1.23.4 auto-upgrade). tinygo (buat .fwpack/WASM agent) di `~/.local/share/tinygo/bin`,
  GOROOT `~/sdk/go1.23.4`. (Lihat memory `flowork-mrflow-build-toolchain`.)
- **Jangan rusak cross-compile:** mobile build `GOOS=android` HARUS tetap sukses — itu gate utama.
- **Deploy:** agent web di-`//go:embed` → ganti GUI butuh rebuild binary. Watchdog (docktor)
  rebuild via `start.sh` pas PID di-kill. Jangan stop docktor lama-lama (dia jaga router+local-AI).

---

## 10. Backup restore-point

Sebelum kerja ini, sistem stabil di-backup ke:
`/home/mrflow/Pictures/flowok_backup/FLowork_os/` (2026-06-23, 23G — source + git + brain-data
7.7G + model 14G; **exclude** `os/out/` karena image OS sudah ada di GitHub Releases).
Restore = `rsync` balik kalau refactor berabe.

---

## 11. Catatan continuity (kondisi saat roadmap ditulis)

- Sesi sebelumnya baru kelar: freeze brain-core (131 hash) + prune zombie (operator-shutdown,
  mr-flow.fwagent stale) + bangun GUI tab Tools + freeze template agent + seed.
- Branch kerja git: `cgm-exec-phase0`. Remote: `origin` (Flowork-OS) + `flowork-base`. Push ke DUA-duanya.
- Roadmap ini **belum** dieksekusi — ini rencana buat kerja berikutnya.

---

## 12. Auto-discovery tool baru + NAMA = kontrak abadi (2 syarat dari owner)

### 12.1 Apakah AI otomatis sadar punya tool baru + cara + kapan pakai?

**"Punya tool + CARA pakai" = OTOMATIS** (terverifikasi):
- Tool ke-`RegisterDynamic` → masuk registry. Kalau di-subscribe ke agent →
  OTOMATIS muncul di tool-specs (nama + deskripsi + schema params, format OpenAI
  function) yang disuntik ke LLM **tiap turn** (`internal/agentmgr/tool_specs.go`,
  cap `maxExposedTools`=52). Ga perlu restart / ajarin manual — **schema ITU cara pakainya**.
- Buat tool di luar 52-exposed: ada meta-tool **`tool_search`** (`internal/tools/builtins/v9_extras.go`)
  — agent nyari KATALOG PENUH by-substring on-demand pas butuh. Jadi long-tail ke-discover sendiri.

**"KAPAN tepat pakai" = 2 lapis:**
1. **Deskripsi tool** (field `Schema().Description`) = "buat apa + kira2 kapan". Deskripsi
   bagus → LLM tau kapan. **→ tiap tool/plugin WAJIB punya deskripsi tajam (kapan dipakai, kapan TIDAK).**
2. **Constitution 5W1H-gate + instinct** = judgment generic anti-asal-ceplos yg berlaku ke
   SEMUA tool (termasuk baru). Judgment DALAM yg spesifik ("pas situasi X pakai tool Y") =
   ditanam sebagai **insting/doktrin** — dan insting itu nyebut tool **by-name** → makanya §12.2.

**Implikasi roadmap:** tiap tool-pack `.fwpack` + tiap tool platform WAJIB punya deskripsi
yang jelas "kapan dipakai". Buat kapabilitas baru yg owner peduli judgment-nya → tambah
1 insting/doktrin (opsional) yg nyebut nama tool-nya.

### 12.2 NAMA TOOL = KONTRAK ABADI (immutable ABI) — syarat keras owner

Owner: "tool yg jadi plugin, nama perintahnya HARUS SAMA — takut udah ke-tanam di insting."

**Aturan:** **nama tool itu kontrak, kayak ABI syscall.** Impl / platform / packaging boleh
ganti; **NAMA JANGAN PERNAH.** Insting/doktrin/skill/pattern nyebut tool by-name → ganti nama
= insting putus.

**Kabar baik (terverifikasi 2026-06-23):**
- Rencana ini **ga ganti nama apapun**: build-tag split cuma misah IMPL per-OS, nama tetap
  (`app_open` ya `app_open` di semua OS). Privileged tool TETAP builtin (cuma di-split), **ga**
  dipindah jadi plugin beda-nama.
- Ada **guard bawaan**: `tools.IsBuiltinName` + `RegisterDynamic` NOLAK plugin yg pakai nama
  builtin (`internal/tools/dynamic.go:38`, `tool_install.go:71`) → nama builtin **ga bisa**
  ke-timpa/ke-shadow diam-diam. Guard ini JUSTRU lindungi insting.
- **Audit protected-names (brain-data):** scan `constitution` + `cognitive_nodes` +
  `mistakes_journal` → **0 nama tool ke-hardcode** di insting/graph. Cuma `brain_search`
  ke-sebut di `router/internal/brain/doctrine_seed.json` (kode). → risiko PUTUS rendah.

**Aturan keras buat eksekusi:**
1. JANGAN rename tool apapun pas refactor (split build-tag = nama sama, impl beda file).
2. Kalau SUATU saat butuh ubah/mindah tool, **rename = HARAM** — bikin tool baru + alias nama
   lama (registrasi 2 nama → 1 impl), JANGAN hapus nama lama.
3. Sebelum nyentuh tool manapun, ulang audit protected-names (code grep doctrine_seed +
   `sqlite3 -readonly flowork-brain.sqlite` LIKE di constitution/cognitive_nodes/mistakes_journal +
   cek persona/self-prompt). Nama yg ke-sebut = HARAM diganti.

---

## 13. BROWSER-CONTROL (computer-use) — terbukti via chrome-devtools-mcp (2026-06-23)

### 13.1 Bukti & sumber (jangan ngarang — udah dicontek dari yg pasti)
Antigravity kontrol browser pakai **`chrome-devtools-mcp`** (MCP server resmi Google, di
`~/Downloads/Antigravity-x64/.../node_modules/chrome-devtools-mcp`). Karena Flowork udah
ngomong MCP (mcphub), kita pasang yang SAMA → **terbukti buka Facebook + baca form login**
(2026-06-23, via mr-flow). 29 tool ke-bridge (`mcp_browser_*`).

**Prinsip (yg dipelajari):**
- Driver = **puppeteer over CDP**. Mode: launch Chrome sendiri ATAU `--browserUrl`/`--wsEndpoint`
  connect ke Chrome jalan (pakai sesi login = cookie).
- Persepsi = **a11y TEXT snapshot DULU** (`take_snapshot` → elemen + `uid`), *"prefer snapshot
  over screenshot"*. Aksi by-uid (click/fill/upload_file). Screenshot cuma buat visual.
- 43 tool total; upload = `upload_file(uid, filePath)` (puppeteer uploadFile/waitForFileChooser).

### 13.2 Build portable/img — MASIH BISA (cek 2026-06-23)
- **IMG (flowork-os):** Chromium **UDAH di-bundle** ([os/build/Dockerfile.rootfs:20](FLowork_os/os/build/Dockerfile.rootfs#L20)
  `apk add cage chromium`, Alpine 3.20 kiosk). Yang kurang cuma node.
- **Portable:** perlu bundle chromium + (node ATAU Go-CDP).
- **Android:** beda total (no node/CDP) → WebView + Accessibility Service (track terpisah).

### 13.3 Dua opsi implementasi (KEPUTUSAN ARSITEKTUR)
- **A. Tetap chrome-devtools-mcp (node):** `apk add nodejs` ke image (+~50MB) + ship paket.
  Plus: proven Google, vocab 29-tool. Minus: runtime node + rantai npm (lawan minimal/standalone).
- **B. Browser-tool Go-native (chromedp/go-rod) — REKOMENDASI buat SHIP:** tulis di Go (drive
  chromium yg udah di image, CDP langsung), **TANPA node**, 1 binary, ikut cross-compile (reuse
  mobile). Cetak-biru = vocab 29-tool + a11y-snapshot dari chrome-devtools-mcp. Minus: tulis sendiri.
- **Strategi:** A buat prototip/sekarang (proven, jalan hari ini) · B buat build yg di-ship.

### 13.4 ⛔ BLOKER PLUG-AND-PLAY — bug cap-grant MCP (HARUS difix duluan)
**Gejala:** install MCP/plugin + subscribe-di-GUI → tool ke-bridge TAPI agent **ga bisa pake**
(`sandbox: capability denied: mcp:<id>`). **Root cause:** `grantSubscribedToolCaps`
([main.go:788](FLowork_os/agent/main.go#L788), FROZEN) jalan pas boot SEBELUM `mcphub.EnableAll`
(async) register tool MCP → `tools.Lookup(name)` balik false → cap `mcp:<id>` ga ke-derive.
Komentar kode sendiri ngaku *"mcp:* ga ke-grant (bug)"* (main.go:600). Ga ada caller runtime;
`SkipCapGate` ga dipake; app-grants cuma `app:`. **→ semua tool MCP saat ini ga kepake lewat
jalur GUI normal.** (Workaround demo: deklarasi cap di manifest agent — BUKAN cara bener.)

**Fix (FONDASI plug-and-play, kerjain DULU):** grant cap MCP setelah tool ke-register.
Cari lokasi EDITABLE (hindari frozen main.go) — kandidat: pasca-`EnableAll`/`Enable` di mcphub
atau helper feature, grant `mcp:<conn>` ke privileged-agent yg subscribe. Juga handle
runtime-install (enable connector → grant), bukan cuma boot. Acceptance: subscribe browser
tool di GUI → `navigate` jalan TANPA manifest-hack.

### 13.5 STATUS 2026-06-23 — EXECUTED ✅ (sprint autonomous, "cabut gigi bukan tambal")
- **Bug cap-grant MCP: FIXED** ([feature_platform.go](FLowork_os/agent/feature_platform.go), commit 25331db).
  Re-grant `grantSubscribedToolCaps` pasca-`EnableAll` (boot) + wrap `/api/mcp/enable` (runtime),
  layer editable, nol sentuh frozen main.go. Bukti: cap mcp:browser ke-grant dari subscribe-GUI →
  navigate Facebook ok tanpa hack. **Plug-and-play foundation jalan.**
- **Opsi B (Go-native browser) DIPILIH + DIBANGUN + TERBUKTI** (cabut gigi, bukan nambal node):
  `agent/internal/tools/builtins/browser_desktop.go` (go-rod, build-tag `(linux||darwin||windows) && !android`).
  8 tool: browser_navigate/snapshot/click/type/upload/screenshot/**set_cookies**/eval. Drive chromium
  yg udah di image, **TANPA node**. Cross-compile: linux=2556 go-rod symbols, **android=0** (ke-exclude bersih,
  mobile aman). Bukti live: navigate Facebook ok + snapshot lihat form login (uid email/pass/login) +
  **cookie-injection terbukti** (inject fw_test → document.cookie kebaca). chrome-devtools-mcp node
  prototype di-UNINSTALL (band-aid dicabut). node user-local masih ada (harmless, bisa dihapus).
- **SISA browser (nanti):** connect-mode test (`FLOWORK_BROWSER_URL` ke Chrome login) · productionize
  (GUI first-class, bukan subscribe manual) · Android (Accessibility Service, track terpisah) ·
  bundle: img tinggal pastiin chromium path (`/usr/bin/chromium`) + cap `browser:control` ke privileged agent.

---

## 14. SIDECAR TOOLS — tool native PLUG-AND-PLAY (folder self-contained) — ✅ MVP DONE 2026-06-23

> Keputusan owner (2026-06-23): biar tool privileged/native jadi **postable** (bukan compiled-in,
> bukan WASM-sandbox), tiap tool = **folder self-contained** yang di-compile jadi **binary native
> SENDIRI** + di-exec host sbg **proses terpisah**.

### 14.1 Prinsip ISOLASI (yang bikin plug-and-play + agnostic)
**Tiap tool = MODUL Go SENDIRI. Dependency-nya DI FOLDER-NYA SENDIRI (own go.mod/vendor). NOL shared
library.** Drop folder → jalan. Ga ada dependency hell, ga nyangkut ke kernel.

### 14.2 Posisi di arsitektur (nutup gap "truly postable")
| | compiled-in | WASM `.fwpack` | **SIDECAR (ini)** |
|---|---|---|---|
| native/privileged | ✅ | ❌ sandbox | ✅ |
| modular (ga sentuh kernel frozen) | ❌ | ✅ | ✅ |
| isolasi proses (crash/jahat) | ❌ | ✅ | ✅ (bisa seccomp nanti) |
| postable upload | ❌ | ✅ semua OS | ✅ desktop (rebuild sidecar); mobile=prebuild + `.fwpack` |

### 14.3 ABI (kontrak abadi — exec stdin/stdout)
Host EXEC binary tool: **STDIN** `{"args":{...}}` → tool kerja → **STDOUT** `{"output":<any>,"error":""}` → exit.
Stateless per-call (proses fresh). CWD = folder tool (boleh baca aset relatif sendiri).

### 14.4 Struktur + flow
```
tools/<name>/{tool.json (manifest), go.mod (modul sendiri), main.go}  → build-tools.sh → binary tools/<name>/<name>
   → host discover (internal/toolsidecar) → tools.RegisterDynamic → ke-expose SEMUA agent → agent exec proses
```
- **Manifest** `tool.json`: `{name, capability, description, returns, params[]}`.
- **Build**: `tools/build-tools.sh` (GOWORK=off, per-modul, cross-compile-aware).
- **Discover/register**: `internal/toolsidecar/toolsidecar.go` (`DiscoverAndRegister`, `Names`, `ToolsDir`).
- **Wiring**: `agent/feature_tools_sidecar.go` (init→RegisterFeature PhaseSeed; main.go frozen GA disentuh).
- **Endpoint**: `GET/POST /api/tools/sidecar` (list / reload tanpa restart host).

### 14.5 AWARENESS + AKSES SEMUA AGENT (syarat keras owner 2026-06-23) — ✅
- **Sadar**: `ToolSpecsHandler` (tool_specs.go) expose `toolsidecar.Names()` ke **SEMUA agent** (sebelum
  subscription = prioritas) → tiap agent liat tool sidecar di spec-nya.
- **Akses semua**: cap `""` di tool.json = **NO gating** (semua agent boleh call, kayak `echo`). Tool
  PRIVILEGED deklarasi cap real (mis. `exec:foo`) → ke-gate broker (cuma agent ber-cap).
- **Bukti live**: `text_hash` ke-expose di mr-flow (66) + fb-writer (ant, 15) + **fb-writer berhasil
  call** → hash bener. (folder `tools/text_hash/`).

### 14.6 SWITCH
- `FLOWORK_TOOLS_DIR` — override lokasi folder tools (default: cari `tools/` deket binary/cwd).

### 14.7 SISA (nanti)
- GUI tab Tools: tampilin sidecar tools + tombol rebuild/reload (sekarang via `/api/tools/sidecar`).
- Build wiring: `tools/build-tools.sh` masuk `agent/start.sh` (auto-build saat compile) + cross-compile Android.
- Keamanan: sandbox OS proses sidecar (seccomp/namespace) buat tool privileged pihak ke-3.
- Contoh tool dgn **dep eksternal vendored** (buktiin lib-di-folder-sendiri end-to-end; butuh network go get).

---

## 15. SELF-EVOLVING TOOLS — agent BIKIN tool sendiri (agent-governed) — 🔭 DESAIN owner 2026-06-23

> Keputusan owner (KRUSIAL): "semua agent bisa bikin tools di `/tools` itu PALING penting". Modelnya
> **self-governed** (BUKAN ACC owner) — biar Flowork **hidup & tumbuh bahkan ketika owner ga ada**.

### 15.1 Pipeline private → review → shared (NOL bottleneck owner)
1. **Tiap tool punya `agent_id`** (pembuat).
2. Agent bikin tool (`tool_create`) → **LANGSUNG jalan TAPI PRIVAT**: cuma si pembuat yang liat (specs)
   + pake (run). GA muncul di search/suggest agent lain. (folder `tools/_private/<agent_id>/<name>/`)
3. **Team-review** (AGENT, configurable kayak self-evolution group — owner pilih model, **GA hardcode**)
   evaluasi: jalan? aman (cek kode anti-jahat)? berguna? non-dup?
4. **Lolos → promote jadi SHARED** (pindah ke `tools/<name>/`, register global, ke-expose semua agent).
   **GA perlu ACC owner** — team yang putusin. (Pola sama cognitive shadow→active→promote + konsensus antibody.)

### 15.2 Komponen (3 fase)
- **FASE 1 — `tool_create` (builtin)**: agent kasih {name, description, params, **LOGIC** (badan fungsi)} →
  host auto-wrap boilerplate ABI + scaffold folder privat + **build-verify** (gagal compile → error balik
  ke agent → benerin → retry = LOOP BELAJAR) → register privat (agent_id+scope=private). Nama unik global
  (anti-collision, §12). Toolsidecar `Spec` nambah `agent_id` + `scope`; ToolSpecsHandler expose private
  HANYA ke owner-nya.
- **FASE 2 — team-review → promote (REUSE DEWAN self-evolution, JANGAN bikin team baru)**: tool privat
  yang mau shared = **evolve-proposal kind `promote-tool`** → ngalir lewat **Dewan ADVERSARIAL yang udah
  ada** (grup `self-evolution`: 🟢Pembela ⚔️ 🔴Penantang → ⚖️Hakim panel-3, gerbang-pilar, konservatif,
  CONFIGURABLE model di GUI — `evolve_council.go` + `evolve_council_group.go` + `selfevolve_group_seed.go`).
  Approve → promote: pindah folder `tools/_private/<agent>/<name>/` → `tools/<name>/` + register shared.
  Cron `runEvolveScheduledCycle` drain backlog → otonom, NO owner-acc. Reuse = konsisten + ga ngarang team.
- **FASE 3 — discovery→edukasi** (dari kasus mr-flow nyari tool ga ada): `tool_search` not-found →
  **semantic recommend** tool mirip (drawer/cognitive) → kalau bener ga ada → **error-edukasi
  `TOOL_NOT_FOUND`** (educational_errors_cache) yang NGAJARIN bikin via `tool_create`. Self-heal: agent ga
  buntu, diajarin tumbuh.

### 15.3 Guardrail (tetap)
- Tool buatan-agent default cap `""` (sandboxed/pure-compute). Privileged (exec/fs/secret) = lewat review
  team yg lebih ketat (bukan auto). Sandbox OS proses (seccomp) = hardening.
- Build cuma desktop (Android ga ada go on-device). Nama = kontrak unik (anti nimpa builtin).
- Review team = agent (configurable), BUKAN owner, BUKAN hardcode → abadi + owner bisa ganti model.

### 15.4 AUTO-GC — seleksi alam buat tool (owner 2026-06-23)
Tiap tool track **`error_count`** + **`last_used_at`** (di DB metadata tool). Cron prune otomatis:
- **error berkali-kali** (> threshold) → tool rusak (mis. API yg dia akses udah berubah) → AUTO-HAPUS.
- **ga kepake N bulan** (default ~3 bln, switch) → obsolete/sementara → AUTO-HAPUS.
Bikin sistem flexible + self-cleaning (ga numpuk tool mati). Pola sama retention cognitive graph + mistakes.
Privat & shared dua-duanya kena GC. (Hapus = folder + unregister; shared yg ke-GC bisa di-recreate kalau perlu.)

### 15.5 LIFECYCLE LENGKAP
`tool_create(privat)` → `dipake (track use/error)` → `Dewan review (promote-tool)` → `shared` →
`[GC kalau error-tinggi / nganggur-lama]` → mati. Tool yg ga survive mati sendiri = evolusi beneran.

### 15.7 DELETION-AWARE — agent SADAR tool mati sampai ke OTAK (owner 2026-06-23, KRUSIAL)
Tool ke-hapus (GC/manual) TAPI otak agent udah "inget" dia (dari pengalaman → masuk lewat **dream**
jadi node/instinct) → agent halu nyoba tool HANTU → error berulang. Hapus dari registry doang = DANGKAL.
Harus MATANG, bersihin sampai cognition:
1. **Unregister** (langsung ga bisa dipanggil).
2. **Bersihin cognitive graph**: hapus/quarantine node `agent:<id>/tool/<nama>` (format terbukti) + edge-nya.
3. **Invalidasi instinct** yang nyebut tool mati ("WHEN…→pake tool X") → quarantine / turunin confidence.
4. **DREAM-GUARD**: `cognitive_dream.go` saat project tool/edge HARUS validasi ke **registry tool LIVE** —
   tool yg ga ada = JANGAN di-(re)project. (Sekarang dream GA validasi → celah halu-tool-hantu.)
5. **Antibody**: tanam penanda "tool X mati, jangan dipake" → agent SADAR beneran, ga nyoba lagi.
Butuh accessor delete/quarantine cognitive node (cek `cognitive_graph.go`; kalau belum ada, tambah —
brain-core FROZEN, butuh izin owner). Ini bikin GC (§15.4) "matang" = lengkap sampai otak.

### 15.6 File evolute relevan (referensi pelaksana)
`selfevolve.go` (proposer) · `internal/agentmgr/evolve_council.go` (CouncilJudge/Verdict + handler) ·
`evolve_council_group.go` (Dewan via 5 agent grup `self-evolution`) · `selfevolve_group_seed.go` (seed grup,
soft-LOCKED) · `internal/agentdb/evolve_pillars.go` (gerbang pilar) · `selfevolve_schedule.go` (cron drain) ·
`internal/agentdb` EvolveProposal + SetEvolveProposalStatus (state machine proposed→approved/rejected).

### 15.8 STATUS IMPLEMENTASI — ✅ SELESAI + TES (2026-06-23)
Semua fase + GC + deletion-aware KEBANGUN & terbukti end-to-end (live localhost:1987):
- **FASE 1** ✅ `tool_create` builtin (cap `""`, di `coreExposedTools` → SEMUA agent) → scaffold privat
  `tools/_private/<agent>/<name>/` (go.mod sendiri, NOL shared-lib = isolasi) + build-verify (gagal →
  `build_log` balik = loop belajar) + anti-eskalasi (tolak os/exec/syscall/unsafe/net). Run-guard:
  tool privat cuma bisa dipanggil pembuatnya (`agentmgr.go`). 6 tes PASS.
- **FASE 2** ✅ promote privat→shared via Dewan: `autoProposePrivateTools` → EvolveProposal `promote-tool`
  → classifier behavior-apply → `promoteToolApply`→`toolsidecar.Promote` (pindah _private→shared). LOOP
  PENUH terbukti: fb-writer bikin `reverse_text` → Dewan → shared → mr-flow liat+pake.
- **FASE 3** ✅ not-found → `toolNotFoundEducation` (`internal/agentmgr/tool_notfound_edu.go`): rekomendasi
  sepadan (token-split `localSuggest` drawer) → ajakan `tool_create`. Plus edu-error DNA via **CABANG**
  `edu_errors_ext.go` (refresh `ERR_TOOL_NOT_FOUND` self-evolving + `ERR_TOOL_GC_REMOVED`) — UPSERT
  override entri frozen `edu_errors_seed.go` (yg DO-NOTHING), nyebar ke semua agent via `ProvisionAgentDNA`.
- **AUTO-GC (§15.4)** ✅ `feature_tools_gc.go` + health-track di `toolsidecar.go` (`error_count`/`last_used`,
  file `.health.json`). `GCScan(maxErr=5, idleDays=90)` → `DeleteTool` (unregister+rm folder+tombstone).
  Ticker 6 jam + `/api/tools/gc`. Switch: `FLOWORK_TOOL_GC_MAXERR/IDLE_DAYS/OFF`. Skip tool bawaan (agentID="").
  Tes: tool `flaky` error 5× → ke-prune (folder GONE + tombstone).
- **DELETION-AWARE (§15.7)** ✅ DUA lapis: (a) PRIMER unregister → ilang dari specs → reactive
  `ERR_TOOL_GC_REMOVED` (cek tombstone) pas agent coba akses. (b) MATANG: `tombstoneSweep` quarantine node
  `agent:<id>/tool/<nama>` (SQL langsung via `store.DB()` — UpsertNode ga update status) + decay confidence
  instinct yg nyebut (×0.3, floor 0.05 → konvergen). Tombstone-based = re-quarantine tiap sweep (nutup
  dream re-project). Tes: inject node+instinct → GC → node `quarantined`, instinct 0.8→0.07.
- **Catatan brain-core**: `edu_errors_seed.go` TETAP frozen (TestKernelFreeze ijo) — perubahan lewat CABANG
  `edu_errors_ext.go` (non-frozen, override). DREAM-GUARD (§15.7 poin 4, validasi project ke registry LIVE)
  = SISA (cognitive_dream.go frozen, butuh cabang) — tapi tombstoneSweep idempoten udah nutup praktis.
