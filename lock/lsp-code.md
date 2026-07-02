# lsp-code — kecerdasan kode SEMANTIK via Language Server (gopls)

Tool `lsp`: query semantik presisi ke language server (gopls, Go) — "level berikutnya"
dari codemap STATIS. definition (go-to), references (find-usages), hover (tipe/dok asli),
lintas-file. Roadmap "buka lock".

## Arsitektur
- File: `agent/internal/tools/builtins/lsp_tool.go` — SIBLING deletable NON-frozen,
  self-register via init() (NOL sentuh builtins.go). Cross-OS (gopls stdio, ga perlu split).
- Klien LSP minimal built-in: JSON-RPC 2.0 over stdio + framing Content-Length,
  correlation id→channel, readLoop skip notifikasi (publishDiagnostics dll). NOL modul
  Go baru (gopls = binary eksternal). initialize→initialized handshake, didOpen per file.
- gopls PERSISTEN per workspace-root (`lspClients` map, reuse) — hemat spawn/index ulang.
- Root modul = go.mod terdekat ke atas (`deriveGoRoot`) biar gopls dapet modul beneran.
- Lokasi symbol: `findSymbolPos` pakai **go/scanner** — cari token IDENT ke-N (skip
  komentar & string otomatis, anti "no identifier found"). char = kolom byte-1 (≈UTF-16 ASCII).

## Keamanan / dep
- **Default OFF** — switch `FLOWORK_LSP=1`. gopls dideteksi (FLOWORK_GOPLS_BIN > PATH >
  GOBIN/GOPATH/~go/bin); ga ada → error sopan (ga crash).
- Isolasi: path via `resolveWorkspaceRel` (workspace-confined, tolak absolut/'..').
  gopls `cmd.Dir` = root di dalam workspace. Capability `code:analyze`.

## Interface
`lsp(file_path, symbol, operation=definition|references|hover, occurrence=1)`:
- definition/references → `results:[{file, line(1-based), snippet}]` + count.
- hover → `hover` (markdown tipe/dok).
- Model nunjuk symbol by NAMA (tool yang cari posisinya) — ga usah ngitung baris/kolom.

## Switch / freeze
- `FLOWORK_LSP` (default OFF) + `FLOWORK_GOPLS_BIN` (override path). Nambah bahasa lain
  (pyright/tsserver) = tool sibling baru pola sama. Status: NON-frozen (deletable).

## QC
build agent+router hijau · vet hijau · unit test LAWAN gopls beneran (hover info tipe,
definition usage→decl, references>=2, reject non-go/'..'/symbol-ga-ada, findSymbolPos
skip-komentar presisi) PASS (skip otomatis kalau gopls ga keinstall) · full builtins nol
regresi · TestKernelFreeze ga kesenggol.
