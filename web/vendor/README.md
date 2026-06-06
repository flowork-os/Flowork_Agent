# static/vendor/ — Bundled Frontend Libraries (Sovereignty)

Per Ayah doctrine 2026-05-05 RULE EMAS + sovereignty: GUI ngga boleh
tergantung CDN external. Library frontend yang butuh JS/CSS bundle
(D3, Mermaid, dll) di-serve dari sini lewat path `/vendor/<file>`.

## Cara Tambah Library

1. Download file `.min.js` / `.min.css` ke folder ini (perlu satu kali
   pakai mesin yang punya internet, atau Ayah copy dari komputer lain).
2. Rebuild flowork-gui binary supaya file masuk ke `embed.FS`:
   ```
   cd floworkos-go
   go build -o build/flowork-gui.exe ./cmd/flowork-gui
   ```
3. Copy ke `bin/flowork-gui.exe` (atau biarin docktor respawn).
4. Refresh browser → library aktif.

## Library yang Dipanggil GUI

| File | Tab/Fitur | Fallback kalau missing |
|---|---|---|
| `d3.min.js` (v7.9.0+) | Code Map graph viz | List view (roots + zombies) |
| `mermaid.min.js` (v10+) | Diagram render di doc viewer | Skip render (tetap text) |

## Kenapa Ngga Auto-Download?

- Sandbox AI agent ngga di-otorisasi download arbitrary URL (sovereignty).
- Ayah explicit kontrol library versions (audit trail + supply-chain
  security).
- Sekali install lokal, project mandiri tanpa internet runtime.

## Source Recommended

- D3.js: https://d3js.org/d3.v7.min.js (atau cdnjs/jsdelivr versi locked)
- Mermaid: https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js

Verify SHA256 setelah download — match versi yang di-test.

## File `.gitignore` di sini

`*.js` + `*.css` ngga ke-track git per default — Ayah punya control
distribusi binary library secara terpisah dari code repo.
