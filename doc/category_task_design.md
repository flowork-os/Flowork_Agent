# Flowork — Arsitektur Global (Request Lifecycle + Category Task)

> Konsep global: dari ENTRY → DISPATCH (pinter: langsung vs LLM) → MR.FLOW (orchestrator)
> → TASK (crew agent fokus) → SYNTHESIZER (ambil keputusan) → USER.
> Inti anti-halu: **prompt kecil per-agent + on-demand fetch + LLM cuma dipakai kalau perlu.**
> Status: DESIGN. Prinsip: buktiin 1 kategori dulu sebelum generalize (anti refactor ke-12).

---

## 1. Peta global (sesuai diagram)

```
 ENTRY POINTS                 DISPATCHER (otak routing)              OUT
 ┌───────────────┐            ┌─────────────────────────────┐
 │ Telegram      │            │ prefix       → command reg   │──┐
 │ CLI           │──┐         │ slash/skill  → DIREX (no LLM)│──┼──► USER (cepat, murah)
 │ TUI           │  │         │ pesan biasa  → inject skill  │──┘   (deterministik)
 │ Quality Ctrl  │  │         └──────────────┬──────────────┘
 └───────────────┘  │                        │ (cuma "pesan biasa" yang lanjut)
 ┌───────────────┐  ├──►  DISPATCHER  ───────┤
 │ AI EXTERNAL   │──┘                         ▼
 │   → MCP       │              ┌─────────────────────────────┐
 └───────────────┘              │  MR.FLOW (orchestrator/LLM) │
 ┌───────────────┐              │  • jawaban simpel → USER    │
 │ SCHEDULER     │──────────────│  • butuh riset? → TRIGGER   │
 └───────────────┘   (bisa      │    TASK CATEGORY            │
                      trigger    └──────────────┬─────────────┘
                      task        ┌──────┬──────┴──────┬──────────┐
                      langsung)  CRYPTO  SAHAM       FOREX      HACKING
                                 [crew]  [crew]      [crew]     [crew]
                                   └──────┴──────────┴──────────┘
                                          ▼  (tiap agent tulis hasil ke /shared/tasks/<run>/)
                                ┌─────────────────────────────┐
                                │ AMBIL KEPUTUSAN BERDASARKAN  │  ← SYNTHESIZER (fan-IN)
                                │ DATA  (baca hasil on-demand) │
                                └──────────────┬──────────────┘
                                               ▼
                                       MR.FLOW → USER
```

---

## 2. Lapisan & tanggung jawab

| Lapisan | Fungsi | Status di repo |
|---|---|---|
| **Entry points** | Telegram / CLI / TUI / Quality-Control — semua masuk ke 1 pipeline. | Telegram + CLI ✅, TUI/QC ⏳ |
| **MCP** | AI eksternal (Claude/Codex/dll) drive Flowork via Model Context Protocol. | ❌ belum |
| **Dispatcher** | Routing PINTER sebelum LLM: prefix/slash → eksekusi LANGSUNG (no LLM, murah+pasti); pesan biasa → inject skill → Mr.Flow. | slash dispatcher ✅, inject-skill ✅ |
| **Mr.Flow (orchestrator)** | Pesan biasa → jawab simpel ATAU trigger Task. BUKAN pelaku analisa. | ada (jadi pelaku) → perlu jadi router |
| **Task Category** | Crew agent fokus (fan-out). 1 agent 1 tugas. | ❌ belum |
| **Synthesizer** | "Ambil keputusan berdasarkan data" — gabungin hasil crew → 1 jawaban (fan-IN). | ❌ belum |
| **Scheduler** | Bisa trigger Task otomatis (terjadwal), tanpa user. | engine ✅, hook ke task ⏳ |

---

## 3. Kenapa ini anti-halu + anti-boros (3 mekanisme)

1. **LLM cuma dipakai kalau perlu.** Prefix/slash command → DIREX (direct exec, NO LLM).
   Hemat token + deterministik + ga ada ruang halu. LLM cuma buat "pesan biasa".
2. **Prompt per worker KECIL.** Tiap agent cuma tau 1 tugas + ≤3 tools-nya. Ga ada katalog tools penuh
   (akar over-prompt). Sisanya on-demand via `tool_search`.
3. **Data antar-agent lewat FILE** di `/shared/tasks/<run>/`, bukan prompt-chaining. Synthesizer baca
   on-demand (ringkas dulu kalau panjang). Context ga numpuk → over-prompt ga balik di step sintesis.

---

## 4. Task Category & crew (dari diagram)

| Task | Agent (1 agent = 1 tugas, fokus) |
|---|---|
| **CRYPTO** | analisa-kode-potensi-scam · analisa-komunitas · analisa-wallet · analisa-teknikal · analisa-fundamental |
| **SAHAM** | analisa-laporan-keuangan · analisa-fundamental · cari-skandal-dirut/ceo · analisa-teknikal |
| **FOREX** | analisa-sentiment-market · analisa-news · analisa-teknikal · analisa-fundamental |
| **HACKING** | cari-job-hackerone · analisa-kode |

> Worker reusable lintas-task (mis. `analisa-fundamental`, `analisa-teknikal` dipakai Saham+Forex+Crypto).
> Bikin via **template**, jangan hand-build satu-satu.

---

## 5. Data model (owner-level, flowork.db)

```
task_categories(id, name, icon, trigger_hint, enabled, created_at)
task_agents(category_id, agent_id, role_label, order_idx, mode['seq'|'par'], optional)   -- crew
task_runs(id, category_id, input_text, status, requested_by, summary, cost_usd, started_at, finished_at)
task_run_steps(id, run_id, agent_id, role_label, order_idx, status, output_ref, cost_usd, ...)
```
Definisi task = owner-level (flowork.db). Worker tetap kerja di **state.db-nya sendiri** (isolated),
hasil di-ekspor ke `/shared/tasks/<run>/<agent>.md`.

---

## 6. Kontrak trigger (reuse yang udah ada)

- Trigger worker: `host.InvokeAgentMessage(agentID, taskPrompt, "taskflow:<run_id>")` (RPC handle_message ✅).
- `taskPrompt` ringkas: `"[TASK] <role> untuk: <input>. Tulis hasil ringkas via file_write ke
  /shared/tasks/<run>/<agent>.md. Fokus tugasmu doang."`
- Budget guard tiap step (finance/budget ✅). Over budget → pause + lapor owner.
- Synthesizer: prompt `"gabungin & ambil keputusan"` + read file output on-demand.

---

## 7. GUI

- **Tab "Tasks"**: list kategori (cards) · edit crew (pilih agent + urutan + mode) · Run (input → timeline live per step + cost + summary). Gaya mirip Threat Radar.

---

## 8. Tooling yang KURANG — bangun DULU (bukan framework)

| Tool | Fungsi | |
|---|---|---|
| `web_search` | cari (Brave/DDG/SerpAPI) | ⭐ wajib |
| `web_archive` | Wayback — berita yang dihapus/dibeli | ⭐ wajib |
| `html_extract` | scrape → teks/tabel bersih | ⭐ wajib |
| `pdf_read` | baca laporan keuangan / filing | tinggi |
| `regulator_fetch` | IDX/OJK/SEC (resmi, anti-manipulasi) | sedang |

> **Darkweb: SKIP.** Risiko legal + isinya lebih gampang dimanipulasi dari berita (disinfo). Nilai
> due-diligence didapat lebih sahih dari web_archive + filing regulator + leak DB publik (ICIJ/OCCRP).

---

## 9. Rencana bertahap (ANTI refactor ke-12)

| Phase | Deliverable | Acceptance |
|---|---|---|
| **0** | Tools: `web_search` + `web_archive` + `html_extract` | agent bisa cari + baca sumber real (ga ngarang) |
| **1** | 1 kategori "SAHAM": crew + synthesizer, via `/shared/tasks/` | **ngalahin single-agent + ga halu** (uji A/B) |
| **2** | Data model + GUI task builder + run timeline | owner bisa bikin/edit/run task dari GUI |
| **3** | Mr.Flow jadi ROUTER (dispatch task vs jawab simpel) + scheduler→task hook | task jalan dari chat + terjadwal |
| **4** | Generalize: Crypto/Forex/Hacking + agent templates + parallel mode | 4 task stabil |
| **5** | MCP server (AI eksternal drive Flowork) + TUI/QC entry | full global concept |

> Buktiin **Phase 1 menang lawan single-agent** dulu. Menang → lanjut. Itu yang nyegah refactor ke-12.
