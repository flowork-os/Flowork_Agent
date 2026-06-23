# tools/ — SIDECAR TOOLS (plug-and-play, ala WordPress)

> Tiap tool = **folder self-contained** = binary native terpisah yang di-exec host. Bikin tool
> privileged/native jadi **modular + terisolasi + postable** tanpa nyentuh kernel frozen.
> Detail arsitektur → `docs/ROADMAP_MULTI_OS_TOOLS.md` (§ SIDECAR TOOLS) + `internal/toolsidecar/`.

## ⚖️ ATURAN ISOLASI (owner Mr.Dev — yang bikin plug-and-play + agnostic)

**Tiap tool = MODUL Go SENDIRI. Dependency-nya ADA DI FOLDER-NYA SENDIRI. NOL shared library.**
Drop folder → jalan. Ga ada dependency hell, ga ada yang nyangkut ke kernel.

## Struktur 1 tool

```
tools/<name>/
  ├── tool.json     # manifest: {name, capability, description, returns, params[]}
  ├── go.mod        # MODUL SENDIRI (+ go.sum + vendor/ kalau ada dep eksternal)
  └── main.go       # program tool (ABI stdin/stdout)
```

## ABI (kontrak abadi — simpel + paling isolasi)

Host EXEC binary tool sbg PROSES TERPISAH:
- **STDIN** (dari host): `{"args": { ...param sesuai tool.json... }}`
- **STDOUT** (ke host): `{"output": <any>, "error": "<string kalau gagal>"}`
- Stateless per-call (proses fresh tiap panggil). CWD = folder tool (boleh baca aset relatif sendiri).

## Cara bikin tool baru (TANPA sentuh kernel)

1. `mkdir tools/<name>` → tulis `main.go` (baca stdin, tulis stdout) + `tool.json` + `go.mod`
   (`module flowork-tool-<name>`). Dep eksternal? `go get` + `go mod vendor` **di folder ini**.
2. `./tools/build-tools.sh` → compile jadi binary `tools/<name>/<name>`.
3. Restart host (atau hit endpoint reload) → host discover + register → tool langsung kepakai agent.

## Switch

- `FLOWORK_TOOLS_DIR` — override lokasi folder tools (default: cari `tools/` deket binary/cwd).

## Catatan

- Tool SIDECAR = NATIVE (lepas sandbox WASM) → buat tool privileged "yang beneran kerja" (shell-ish,
  file, dll). Cap di `tool.json` nge-gate agent mana yang boleh (broker). Sandbox OS proses (seccomp/
  namespace) = lapis keamanan masa depan.
- Mobile (Android): sidecar di-PREBUILD ke APK (cross-compile, ga bisa rebuild on-device). Add-on
  upload di mobile pakai WASM `.fwpack` (sandboxed). Desktop = rebuild sidecar (truly postable).
