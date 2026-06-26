# LOCALAI RUNTIME — model lokal flowork-brain (GUI Mesh & Policy · Section 25)

> Dev: Aola Sahidin. 2026-06-26. Mesin LLM lokal berdaulat: `flowork-brain.gguf` jalan di
> llama.cpp (`llama-server` :8088). Cara kerja: `os/`.

## MODEL: flowork-brain (BUKAN qwen)
- **flowork-brain** = otak lokal Flowork (gguf ~16.8GB di `router/models/flowork-brain.gguf`), jalan
  di `llama-server` :8088, dipanggil router via provider `http://127.0.0.1:8088/v1` (di-seed di
  `seed/router-config.seed.json`, format openai, tags: local/sovereign/uncensored).
- Model **resolve via** `localai/runtime.go` → `ResolveFloworkBrain()` → `sidecar.ModelGGUF()`
  (`FloworkBrainModel = "flowork-brain"`). Const + path = code, BUKAN dari registry DB.

## JALUR
```
GUI "LocalAI Runtime" (Sec 25, index.html) → /api/localai/{models,runtime,autostart-toggle}
  → LocalAIModelsHandler (handlers_llm_policy.go): GET list / POST upsert ke tabel localai_models
  → runtime start: ResolveFloworkBrain() (runtime.go) → spawn llama-server (resolver, bukan registry)
Autostart: checkbox "Auto-start local AI (flowork-brain) saat router boot".
```

## TABEL localai_models = DAFTAR DISPLAY (bukan sumber model jalan)
- Registry `localai_models` cuma list buat panel (nama/path/size). Model BENERAN jalan via resolver
  (`ResolveFloworkBrain`), jadi flowork-brain tetap jalan walau registry kosong.
- **Tidak ada DELETE endpoint** (cuma GET + POST upsert). Stale row = perlu cleanup manual/owner.

## FIX 2026-06-26 — registry nampilin qwen-7b basi → flowork-brain
**Akar:** registry punya 1 row `qwen-7b` (checksum placeholder `sha256:abcd`, path `/var/lib/flowork/...`
yg gak ada) = **data DB basi dev ini doang** (GAK di kode/seed → fresh-install gak kena). flowork-brain
gak ke-list (registry cuma display, model jalan via resolver).
**Fix (GUI non-frozen + API):**
1. flowork-brain di-register via `POST /api/localai/models` (id=2, path+size real). Idempotent upsert.
2. GUI (`index.html`): registry **saring** baris demo-seed (`checksum === 'sha256:abcd'`) + input model
   **pre-fill `flowork-brain`**. → panel nampilin flowork-brain doang.
- ⚠️ qwen-7b row masih ada di DATA (gak ada DELETE API + tulis-DB-live diblok safety) — cuma
  DISEMBUNYIIN di GUI. Hapus beneran = butuh endpoint DELETE (unfreeze handler) atau cleanup DB owner.

## FROZEN (logic) vs SEAM (GUI)
- **FROZEN**: `internal/localai/runtime.go` (resolver+spawn), `handlers_llm_policy.go` (LocalAIModelsHandler),
  `internal/store/llm_pricing_policy_migrations.go` (tabel localai_models).
- **SEAM (non-frozen)**: `web/static/index.html` (GUI — sesuai prinsip: lock logic, GUI biarin),
  `seed/router-config.seed.json` (provider seed flowork-brain).

## VERIFIKASI 2026-06-26
QC live (:2402, restart): registry API 2 row (qwen+flowork-brain), GUI nampilin **flowork-brain doang**
(demo disaring). Input pre-fill flowork-brain. Model jalan (llama-server :8088, flowork-brain.gguf 16.8GB).
Build router OK.
