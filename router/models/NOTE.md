# router/models — local model weights (GGUF, NOT committed)

GGUF weight files for the local sovereign model. They are **multi-GB** (over GitHub's 100 MB file
limit), so they are **git-ignored**; only this note (and `README.md`) is tracked.

## The model: `flowork-brain`
- `flowork-brain.gguf` — the local sovereign brain: a **gemma-based** model (gemma 4 26B-A4B MoE,
  quantized) that Flowork runs on-device, fully offline. Branded **flowork-brain** in the router.
- `mtp-gemma-4-26B-A4B-it.gguf` — optional MTP / draft weights for speculative decoding.

## How to get it
1. Download a GGUF build of the model sized for your hardware (a Q4 quant fits ~8 GB VRAM with
   CPU-MoE offload; more VRAM → larger quant).
2. Place it here as `flowork-brain.gguf`.
3. The router resolves it via `$FLOWORK_BRAIN_GGUF` → `<exe-dir>/models/flowork-brain.gguf`
   (see `ResolveFloworkBrain`); `internal/localai/runtime.go` launches `llama-server` with it when
   `FLOWORK_LOCALAI_AUTOSTART=1`.

Hardware tuning (GPU offload, KV-cache quant, context size) lives in `../flowork.local.env`
(owner-local, git-ignored) — e.g. `FLOWORK_NGL`, `FLOWORK_CPU_MOE`, `FLOWORK_KV_TYPE`, `FLOWORK_CTX`.
