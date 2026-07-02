# notebook-edit — tool edit Jupyter .ipynb per-cell

Tool `notebook_edit`: agent bisa ngedit notebook Jupyter per-cell (replace source,
insert cell, delete cell) tanpa nulis-ulang seluruh file. Roadmap "buka lock".

## Arsitektur
- File: `agent/internal/tools/builtins/notebook_edit.go` — SIBLING deletable NON-frozen.
  Self-register via `init(){ tools.Register(&notebookEditTool{}) }` (pola engine_scan_ext).
  **NOL sentuh `builtins.go` frozen.** Hapus file → tool ilang, core utuh.
- Capability `fs:write:/shared/*` (sama kelas file_write → warisan izin sama).
- Path lewat `resolveFileArgs` (file_path relatif, workspace-confined). Isolasi terjaga:
  absolut / `..` / drive Windows ditolak (doktrin ERR_WORKSPACE_ESCAPE). Wajib `.ipynb`.

## Perilaku
- `edit_mode=replace` (default): timpa `source` cell target (by `cell_index` 0-based
  ATAU `cell_id` nbformat 4.5+). Cell code → `outputs` & `execution_count` DIRESET
  (Jupyter semantics: source berubah = output lama basi).
- `edit_mode=insert`: sisip cell baru (`cell_type` code|markdown, default code) di
  `cell_index` (default append di ekor). Code cell dapet outputs:[] + execution_count:null.
- `edit_mode=delete`: hapus cell target.
- Notebook di-unmarshal ke map generik → SEMUA field nbformat (metadata/kernelspec/
  nbformat_minor/outputs) kepreserve pas ditulis balik (ANTI-KORUP notebook user).
- Balikin ringkasan cell (index/type/id/preview) biar model tau state baru.

## Switch / freeze
- Ga ada switch env (tool murni; ada/nggak = ada/nggak file). Nambah kapabilitas
  notebook lain = tool sibling baru.
- Status: FROZEN 2026-07-02 (seizin owner, stabil+live). Tool baru = sibling baru (ga usah unlock).

## QC
build agent hijau · vet hijau · unit test (replace-reset-outputs + keep-nbformat-meta,
insert+delete by id, reject non-ipynb/out-of-range/`..`-escape, registrasi papan) PASS ·
TestKernelFreeze ga kesenggol (nol frozen).
