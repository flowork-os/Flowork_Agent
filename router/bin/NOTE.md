# router/bin — native llama.cpp libraries (NOT committed)

This folder holds the compiled **llama.cpp** shared libraries the local-model runtime loads
(`llama-server` + GGML backends). They are **build artifacts** — per-OS / per-GPU — and often
exceed GitHub's 100 MB file limit (`libggml-cuda.so` ≈ 200 MB), so they are **git-ignored** and
shipped per-OS via release bundles, not committed here. Only this note is tracked.

## Files that belong here
- `libggml-base.so`, `libggml-cpu.so`, `libggml.so` — GGML core + CPU backend
- `libggml-cuda.so` — CUDA backend (NVIDIA GPUs only; large)
- `libllama.so`, `libllama-common.so`, `libllama-server-impl.so` — llama.cpp + server impl
- `llama-server` — the server binary the router launches (see `internal/localai/runtime.go`)

## How to install / build

### Option A — download a prebuilt llama.cpp release (fastest)
1. Grab a build matching your OS + accelerator from <https://github.com/ggml-org/llama.cpp/releases>
   (CUDA build for NVIDIA, Metal for macOS, CPU/Vulkan otherwise).
2. Copy the `.so` / `.dylib` / `.dll` libraries **and** the `llama-server` binary into this folder.

### Option B — build from source (matches your exact hardware)
```sh
git clone https://github.com/ggml-org/llama.cpp && cd llama.cpp
cmake -B build -DGGML_CUDA=ON          # NVIDIA; use -DGGML_METAL=ON on macOS, omit for CPU-only
cmake --build build --config Release -j
# then copy build/bin/libggml*.so  build/bin/libllama*.so  build/bin/llama-server  ->  this folder
```

The router auto-detects the binary via `$FLOWORK_LLAMA_BIN` → `<exe-dir>/bin/llama-server` → `PATH`
(see `ResolveLlamaBin`). With these libraries present plus a model in `../models`, the local
sovereign brain runs fully offline.
