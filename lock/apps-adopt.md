# 🌉 APPS-ADOPT — Jembatan "Repo → App" (Sidecar)

Arsitektur fitur **adopt**: repo mentah (git-URL / folder) → app yang dijalanin **MANUSIA (GUI) & AI (tool)**,
tanpa build manual. Reuse penuh substrat apps (`runtime:process`, manifest, reloadOne, app_grants).

## Inti
Repo mentah ga ngerti protokol core Flowork. Jembatannya = **CLI-Adapter Core generik** (`fw-app-adapter`):
1 binary, ngomong protokol stdio (`proc.go`: `{op,args}` ↔ `{result,state_version}`), nerjemahin tiap `op`
→ command repo yg dipetakan di `adapter.json`. Engine `runtime:process` jalanin adapter sbg core → **nol ubah engine**.

```
clone/copy repo → deteksi runtime → install dep KE FOLDER → tulis manifest.json + adapter.json → reloadOne → LIVE
  app/<id>/repo/        (kode + venv/node_modules/target = dep lokal; hapus folder = bersih)
  app/<id>/adapter.json (workdir "repo" + ops: run→RunCmd, arg_style args_list)
  app/<id>/manifest.json(runtime:process, core_entry=<fw-app-adapter>, op run tool:true → tool agent app_<id>_run)
```

## File (semua SEAM — nol file frozen lama disentuh)
| File | Peran | Status |
|---|---|---|
| `internal/apps/cliadapter/adapter.go` | **CORE** adapter: loop stdio + exec argv (no shell) + placeholder/flags/args_list/json_stdin + resolve program relatif ke workdir + timeout | **LOCKED** (hash) |
| `cmd/fw-app-adapter/main.go` | binary core_entry (cwd=folder app) | **LOCKED** (hash) |
| `internal/apps/adopt/detect.go` | **CORE** deteksi runtime (python/node/go/rust) + **registry switch** `RegisterDetector` (POLA A: runtime baru via sibling, NOL unfreeze) | **LOCKED** (hash) |
| `internal/apps/adopt_ext.go` | orchestration `AdoptRepo`/`DetectSource` (sibling apps; panggil reloadOne) | non-frozen (growth) |
| `internal/apps/adopt_fsutil_ext.go` | util fs/json (copyTree, writeJSON) | non-frozen |
| `feature_app_adopt_ext.go` | SEAM route `/api/apps/adopt` + `/api/apps/detect` (init→RegisterFeature) | non-frozen (deletable) |

## Switch / evolusi (Rule #7)
- **Runtime baru** (ruby/php/deno/dotnet…) → sibling `init(){ adopt.RegisterDetector(...) }`, ga sentuh `detect.go`.
- **Kontrak baru** (HTTP/MCP, roadmap F5) → adapter/contract BARU (binary/sibling), bukan edit CLI-adapter.
- Hapus `feature_app_adopt_ext.go` → fitur adopt mati mulus, core utuh (self-sufficient).

## Keamanan
Consent exec WAJIB (`?approve_exec=1`) — clone+install = perintah OS, owner buka gerbang (bukan AI). Dep di folder
(isolasi). Path adapter di-resolve runtime (no-hardcode, multi-OS). White-label (nol identitas corporate). Scanner
pre-flight + tier-isolasi per-OS = roadmap F6.

## Verifikasi (litmus LULUS 2026-06-27)
HTTP `detect`→`adopt` → app LIVE · `/api/apps/op run` (manusia) → `LIVE-ADOPT-OK` · mr-flow (bahasa-manusia) →
"Outputnya: `LIVE-ADOPT-OK`". E2E deterministik (adopt repo Go → build → InvokeOp + args) PASS. `TestKernelFreeze` PASS.

## Build wiring fw-app-adapter (wajib sebelah flowork-agent, resolve via adapterBinPath)
- ✅ **portable** (`os/portable/make-portable.sh`): build per-OS + copy ke install bin + chmod.
- ✅ **appliance** (`os/build/build-flowork-os.sh`): build static + install `/usr/local/bin/fw-app-adapter`.
- ⏳ **dev** (`agent/start.sh`): DITUNDA — start.sh ada WIP owner. Edit yg perlu (idempotent, sebelah build flowork-gui):
  `( cd "$ROOT" && CGO_ENABLED=0 go build -o "$ROOT/bin/fw-app-adapter" ./cmd/fw-app-adapter )`.
  (Sementara: dev jalan krn binary udah di-build manual ke agent/bin/.)

## Belum (roadmap)
- `chattr +i` 3 file LOCKED (OS-immutable layer-2) — butuh sudo dev.
- F4 GUI panel Adopt · F5 kontrak HTTP+MCP · F6 scanner+tier-isolasi.
