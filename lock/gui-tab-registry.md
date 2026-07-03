# 🔌 SWITCH GUI — Tab Registry (stop-kontak sebelum semua GUI dibeku)

> Owner 2026-07-03: SEMUA GUI dibeku. Biar nambah tab ga bobok tembok beku, dipasang stop-kontak DULU
> (rule emas §5/§7). Pola-A (papan colokan, default kosong = aman).

## Cara nambah TAB GUI baru TANPA buka file beku
1. Bikin sibling `agent/gui_tab_<x>_ext.go` (deletable):
   ```go
   package main
   func init() { RegisterGUITab(GUITabSpec{Name:"x", Icon:"🔧", Label:"X", LabelKey:"x.title", I18nDomain:"x", Order:50}) }
   ```
2. Bikin `agent/web/tabs/x.js` (deletable) dengan `export function render(main){ ... }`.
3. (Opsional) i18n: `agent/web/i18n/en/x.json` + `id/x.json` (deklarasi `I18nDomain:"x"` → auto-muat).
→ Boot ulang: tab muncul di nav. **NOL edit file beku.** Hapus 2-3 file itu → tab ilang, GUI utuh.

## Mekanisme (yang DIBEKU)
- `agent/gui_tab_registry.go` — papan colokan: `RegisterGUITab()` + endpoint `GET /api/gui/tabs-ext`
  (default registry KOSONG → `{tabs:[]}` → nav = builtin doang = aman).
- `web/js/app.js` — `initExtTabs()`: pas boot fetch `/api/gui/tabs-ext` → APPEND tombol nav + `ACTIVE_TABS`
  + muat i18n domain. Builtin nav (index.html) & app.js builtin NOL disentuh. `loadTab()` udah dinamis.
- `web/js/i18n.js` — `loadDomain()`: muat domain i18n ekstensi on-demand (domain ekstensi ga perlu masuk `DOMAINS`).

## Yang UDAH ada seam-nya (ga butuh ini)
- Switch setting baru → `fwswitch.Registry` + `/api/settings/switches` (settings.js data-driven).
- Isi/konten tab → `tabs/*.js` (deletable). Fitur backend → `RegisterFeature`.

## Bukti (delete-test, 2026-07-03)
Colok dummy `gui_tab_demo_ext.go` + `tabs/demo-ext.js` → `/api/gui/tabs-ext` munculin `demo-ext`, tab js
keserve 200. Hapus dua-duanya → build OK, `{tabs:[]}`, GUI utuh (index.html+app.js 200). LULUS.

## Yang belum ke-seam (jarang, conscious-unfreeze aja)
- API-key preset baru (`settings.js` KEY_PRESETS) — jarang; kalau sering, bikin `RegisterKeyPreset` mirip.
- Edit STRING/logika tab yang UDAH ada = unfreeze sadar (bukan pertumbuhan). Seam ini buat tab BARU.
