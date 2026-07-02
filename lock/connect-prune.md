# connect-prune — rapihin daftar Connect Provider / OAuth Imports / CLI Tools

Owner cuma mau lihat yang **proven** (kepasang/ke-login), sembunyiin CLI/preset yang
belum bisa dites. Diatur 3 SEAM post-filter — inti (papan colokan) BEKU, filter-nya
diisi sibling `_ext.go`. Semua bisa dimatiin lewat switch env (GUI = kebenaran).

## Arsitektur (3 seam, pola B: var func default nil = apa adanya)

| Seam (var, di file BEKU) | Diisi (sibling BEKU 2026-07-02) | Switch mati |
|---|---|---|
| `clitools.DetectFilter` — `internal/clitools/detect.go` | `internal/clitools/detect_filter_ext.go` | `FLOWORK_CLITOOL_PRUNE=0` |
| `creds.DetectFilter` — `internal/creds/imports.go` | `internal/creds/imports_antigravity.go` | `FLOWORK_IMPORT_PRUNE=0` |
| `PresetsHook` (main) | `presets_filter_ext.go` | `FLOWORK_PRESET_PRUNE=0` |

- `DetectFilter` dipanggil di ujung `DetectAll()`; `nil` = daftar apa adanya; panic ext
  di-`recover` (fail-safe → daftar penuh). Balikin `nil` juga = apa adanya.
- Kriteria SIMPAN: **CLI Tools** = `Installed || SettingsExists || HasFlowRouter || TokenSet`.
  **OAuth Imports** = `Found`. **Preset** = bukan di `hiddenPresetIDs`.

## Perilaku terkunci

- CLI Tools GUI: cuma tool yg ada jejak (live `count=5`: claude, cline, dst).
- OAuth Imports GUI: cuma sumber `found` (live `count=2`). `gemini-cli` detektor
  di-comment (owner ga punya buat dites; re-enable = uncomment → butuh unlock).
- Connect Provider: `hiddenPresetIDs` = kiro-ai, opencode, codeium-plus, windsurf-cascade,
  jetbrains-ai, zed-ai (CLI-login untested). Antigravity di-inject paling atas.
- **Antigravity preset `AuthType = subscription`** (bukan api_key): login OAuth via tab
  OAuth Imports, NO API key diketik → muncul di filter GUI "Subscription" (sekelas
  Claude Pro/Max), form connect ga salah minta key. Provider RUNTIME asli tetap dibikin
  backend (`ensureAntigravityProvider`, AuthTypeAPIKey + token auto-capture) — independen
  dari authType preset (preset cuma kartu UI). Filter GUI: `index.html` (`authType==='subscription'`).

## Status file

| File | Status |
|---|---|
| `internal/clitools/detect.go` (seam board) | LOCKED (sejak awal) |
| `internal/creds/imports.go` (seam board) | LOCKED (sejak awal) |
| `internal/clitools/detect_filter_ext.go` | LOCKED 2026-07-02 |
| `internal/creds/imports_antigravity.go` | LOCKED 2026-07-02 |
| `presets_filter_ext.go` | LOCKED 2026-07-02 |

Nambah filter/preset baru TANPA unlock: bikin sibling `_ext.go` baru yg ngeset seam
(chain), atau pakai switch env. Ubah struktur (unhide preset / re-enable gemini-cli) =
butuh `sudo chattr -i` → edit → re-hash → `chattr +i`.
