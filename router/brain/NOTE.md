# router/brain — the shared knowledge brain (data NOT committed)

This folder holds **`flowork-brain.sqlite`** — the router's shared Memory Palace (millions of
"drawers" + a quantized semantic vector index). It is **tens of GB**, far over GitHub's 100 MB
file limit, so the data is **git-ignored**; only this note is tracked.

## What lives here
- `flowork-brain.sqlite` — the knowledge corpus (drawers + FTS5 keyword index + Quantum vector index).
- `brain.vindex` — the quantized (8-bit) semantic index, built by the RAG pipeline under `_rag/`.
- The local model that reads/writes this brain is **`flowork-brain`** — a **gemma-based** model
  (gemma 4 26B-A4B, quantized), stored separately as `../models/flowork-brain.gguf`.

## How to get it
- **Fresh install:** the brain auto-creates an empty schema on first run — Flowork starts learning
  from zero. No download is required to boot.
- **Restore a populated brain:** drop your `flowork-brain.sqlite` backup into this folder. The router
  resolves it via `$FLOW_ROUTER_BRAIN_DB` → `<exe-dir>/brain/flowork-brain.sqlite`.
- Because the data is too large for git, keep backups on **external storage** (e.g. Google Drive or a
  local backup disk), not in the repository.

The brain is **portable** — copy the folder and the whole memory travels with it.
