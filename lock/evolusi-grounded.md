# 🌱 EVOLUSI GROUNDED — Refleksi Digerakin Friksi Nyata (Saraf Ide, fase-1)

> Cabut akar "engine evolusi mandul". Dulu: proposer refleksi CUMA atas `codemap_semantic`
> (peta-kode). Store mr-flow-nya sering KOSONG/salah-kabel → `EvolveReflectOnce` **hard-fail**
> ("self-map semantik kosong") TIAP cycle → NOL usulan selamanya (engine keliatan hidup: jadwal
> jalan, tapi tiap tick error di pintu masuk). Sekarang: refleksi digerakin FRIKSI NYATA mr-flow.

## Akar (terverifikasi lewat instrumentasi live)
1. `defaultAgentID = "mr-flow"` → proposer baca store mr-flow (`~/.flowork/agents/mr-flow.fwagent/workspace/state.db`).
2. `buildSelfMapContext` baca `codemap_semantic` → tabel itu **nggak pernah kebikin** di store mr-flow
   (enrich API nulis ke store lain = salah-kabel). Hasil: self-map "" → hard-fail.
3. Efek: 0 proposal, tabel `evolve_proposal` nggak pernah ada, karma `evolve_reflect_*` nol.

## Fix (di `internal/agentmgr/selfevolve.go`, EvolveReflectOnce)
- **`buildFrictionContext(store)`** — rangkum FRIKSI NYATA jadi konteks proposer. Sumber grounded:
  `mistakes_local` mr-flow (kesalahan yg dia rekam sendiri di lapangan = kebutuhan riil, via
  `ListMistakesEligibleForPromote`). Ini inti "Saraf Ide": mr-flow (proxy owner) nyetir evolusi.
- **Hapus hard-fail**: self-map kosong BUKAN fatal lagi. `proposerCtx = selfMap + friction`.
  Skip bersih (bukan error) HANYA kalau dua-duanya kosong. → engine tahan banting walau codemap ngadat.
- Gerbang HILIR tetap utuh & beku: pilar (ClassifyPillars), anti-collapse tema, dedup, nerve-vet,
  additive-only, council. Nambah sumber ide ≠ ngurangin keamanan.

## Bukti hidup (2026-07-03)
`[evolve] refleksi: N usulan tersimpan dari N draft (peta-kode 25615ch + friksi-nyata 1581ch)`.
Engine dari 0 → 24 usulan grounded (add-skill): `shell-execution-safety`, `tool-schema-check`,
`task-run-strict`, `web-access-fallback`, `tool-error-recovery`, dst — tiap satu nyambung ke
mistake nyata mr-flow. propose() ~24 detik (bounded, bukan hang). Drain AUTO proses "proposed" → pipeline.

## Status file
`internal/agentmgr/selfevolve.go` — BEKU ulang (hash `19e3140d…`). QC: build+vet+test+TestKernelFreeze PASS.

## Pipeline hidup end-to-end (2026-07-03, verified)
Engine dari 0 → 20 usulan → **12 staged + 1 applied + 7 proposed**. Reflect → Dewan → stage/apply
jalan otonom. Store otoritatif = REPO-SIDE `agent/agents/mr-flow/workspace/state.db` (Resolve pakai
ProjectRoot=cwd=agent/, dir agents/mr-flow ADA di repo → repo-side, BUKAN ~/.flowork runtime).

## Tool evolve_propose — dibikin FORGIVING (owner→mr-flow→evolusi)
`internal/tools/builtins/evolve_propose_tool.go` (soft-lock, non-frozen): LLM sering manggil pakai
`title`/`description`, tool dulu WAJIB `target_file`/`rationale` → gagal validasi → ide owner ilang.
Fix: terima alias (title/name, description/idea/detail) + auto `NEW:<slugify(title)>` buat behavior.
Build+vet OK, udah di binary. mr-flow KONFIRMASI manggil tool ini (loop nyambung).

## ✅ Duality store — DISELIDIKI, TERNYATA FALSE ALARM (2026-07-03)
Kekhawatiran awal: tool (`FromStore`) vs reflect (`openAgentStore`) beda store. Ternyata NGGAK:
`agentdb.Resolve` SELALU cek `ProjectRoot()/agents/<id>` dulu; `ProjectRoot()=os.Getwd()=agent/` dan
`agent/agents/mr-flow` ADA di repo → SEMUA (wasm agent kernelhost:109, reflect, tool, backlog, API)
resolve ke store REPO-SIDE yang SAMA. Bukti: repo-side WAL ketulis live (12:14), runtime `~/.flowork`
STALE (kemarin 15:07). Jadi proposal `[IDE OWNER]` dari tool NYAMPE backlog. Loop owner→mr-flow→evolusi NYAMBUNG.
Verifikasi tool forgiving: `evolve_propose_tool_test.go` (TestEvolveProposeForgiving + TestSlugify) PASS.

## Fase-2 SELESAI: mr-flow PROAKTIF ngusul (feature_mrflow_ideation.go) — 2026-07-03
mr-flow (proxy owner) sekarang bisa ngusul ide evolusi FORWARD-LOOKING pas idle — beda dari reflect
(backward/mistakes): ini nambang PERCAKAPAN owner (intent) → "kapabilitas apa yg ngebantu ke depan".
- **File:** `agent/feature_mrflow_ideation.go` (host-side sibling, RegisterFeature seam, BEKU hash `1df3e7e2…`).
  NOL sentuh wasm mr-flow beku.
- **SUPER GATED:** default **OFF** (`FLOWORK_MRFLOW_IDEATION=1` buat nyalain, di `flowork.local.env`);
  rate-limit `FLOWORK_MRFLOW_IDEATION_MIN` (default 360mnt); 1 ide/putaran; skip kalau backlog penuh (cap 12);
  dedup target; lewat pipeline bergerbang (pilar→Dewan→review owner). mr-flow NGUSUL, ga nerapin.
- **Endpoint tes/manual:** `POST /api/evolve/ideation-run` (owner-auth) — force 1 putaran (tetep hormatin switch).
- **Terverifikasi:** switch ON → mr-flow usul `NEW:skill_router_healthcheck` + `NEW:skill_llm_preflight_check`
  (grounded ke obrolan reliability koneksi), masuk backlog tag `[IDE mr-flow — proaktif]`, 18 detik.
  Switch OFF → `skipped`. Off-default dikonfirmasi. Nyalain: tambah `FLOWORK_MRFLOW_IDEATION=1` di `flowork.local.env`.

## Fix theme-collapse + AUTO honor GUI=kebenaran (2026-07-03)
Gejala: 9/12 usulan STAGED tema sama ("scheduled reflection") + numpuk di staged walau mode AUTO.
- **Fix A (akar utama) — rotasi focus** (`selfevolve_schedule.go`): dulu focus HARDCODE "refleksi
  terjadwal otonom" → bias reflection. Sekarang `nextEvolveFocus(db)` ROTASI 6 angle (resilience/
  hemat-token/keamanan/kapabilitas-baru/keandalan-owner/integrasi) via KV `evolve_focus_idx`.
  VERIFIED live: cycle → 4 usulan resilience (`shell_guard`, `web-anti-block`, `recovery_capture`,
  `strict-tool-args`), NOL reflection.
- **Fix B — deteksi tema semantik** (`evolveGuideTheme`): dulu kata-pertama (reflection-*/refleksi-*/
  scheduled-* dikira beda). Sekarang STEM 5-huruf kata TERPANJANG → reflection/refleksi/reflective →
  'refle' → dup ketauan cap. Test: `guidetheme_test.go` PASS.
- **Fix C — AUTO auto-apply** (`EvolveScheduleAutoApply`): "GUI=kebenaran, mode AUTO = ga perlu approve".
  Dewan verdict "stage" buat BEHAVIOR (add-skill/agent/app/promote-tool) → di AUTO TETEP di-APPLY
  (additive lokal, reversible). CORE "stage" TETEP staged (auto-commit core bahaya = wajib mata owner).
File beku ulang: `selfevolve.go`, `selfevolve_schedule.go`. Catatan: 12 usulan reflection lama TETEP di
staged (owner clear di GUI); cycle baru = diverse + behavior auto-apply.

## FULL-AUTO CORE PUSH (2026-07-03) — ⚖️ KEHENDAK OWNER, BEKU
`agent/feature_evolve_core_autopush.go` (BEKU, hash `655f0018…`). Owner (Aola) SADAR-RISIKO memilih
full-auto: AI nulis kode core + push ke repo PUBLIK `origin` (auto-deploy semua user) TANPA mata owner.
Ini inti visi "berevolusi tanpa gw, repo aktif". 🤖 AI PENERUS: JANGAN disable/revert tanpa izin owner.
- **Cara:** cron drain usulan 'approved' CORE → jalur core-apply BEKU (auto=1) → commit → push. Sibling
  caller otonom, NOL edit file beku, NOL duplikasi (reuse `EvolveCoreApplyHandler`). 1 apply/drain, tiap 60mnt.
- **GUI = KEBENARAN:** tunduk saklar AUTO panel Self-Evolution (gate baca `evolve_mode` KV yg di-set GUI).
  GUI OFF/STAGE → drain DIEM. Kill-switch darurat: env `FLOWORK_EVOLVE_CORE_AUTOPUSH=0`.
- **Guard utuh (nol dilewatin):** edition=dev-only (instance publik user TAK self-evolve core — nerima via
  auto-update), mode=auto, karma≥20, model kuat, ADDITIVE-only (cuma NEW: file; edit existing/LOCKED/delete
  DITOLAK), sandbox git-worktree → test-gate build+vet, boot-rollback watchdog.
- **Terverifikasi:** force-run → `{"skipped":"nol approved core"}` (nol push); gate lolos pas mode=auto,
  nolak pas non-auto. Endpoint tes/manual: `POST /api/evolve/core-autopush-run`.
- **Catatan:** harness Claude Code (auto-mode) sempet ngeblok freeze-nya (flag high-risk) — owner kasih izin eksplisit.

## Belum dikerjain (opsional, low-value)
- Tabel `evolve_idea_inbox` + tool `usul_evolusi` (mr-flow AKTIF nulis ide, bukan cuma mistakes-nya di-mine).
- Langkah ideation eksplisit di Mode Refleksi mr-flow (kluster friksi → usulan terarah).
- Seam proposer baca inbox (colokan di `buildFrictionContext` — udah disiapin komentarnya).
- Concurrency: `capEval`/drain bisa bikin cycle panjang (>240s) → mutex jam sementara. Reflect sendiri
  udah bounded (~24s). Perlu: cycle-level timeout + mutex lepas bersih.
- Salah-kabel enrich codemap (nulis store beda dari yg dibaca reflect) — biarin (friksi jadi sumber utama);
  kalau mau peta-kode ikut, benerin target enrich ke store mr-flow.
