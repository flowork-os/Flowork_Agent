# COGNITIVE GRAPH (CGM) — Twin Graph per-Agent: Cara Kerja, Kontradiksi, Tools & Switch

> Dokumen referensi (white-label). Menjelaskan SEMUA soal Cognitive Graph: cara kerja, model
> node/edge, masalah orphan + fix limit, kontradiksi (tension) + loop klarifikasi, tools,
> switch/cabang, dan cara nambah filtur. Owner: Aola Sahidin (Mr.Dev).
> Repo: https://github.com/flowork-os/Flowork-OS. Update: 2026-06-23.
> ⚠️ File ini KE-TRACK repo → NOL data personal owner (mekanisme generic doang).

---

## ⛔ WAJIB BACA DULU (buat AI/dev yang mau ngedit)

File CGM di bawah ini **DI-FREEZE** (chattr +i + hash `KERNEL_FREEZE.md`). **JANGAN buka.**
Mau nambah/tuning filtur CGM:

1. **Tuning limit viz / switch** → env yang dibaca **`internal/agentmgr/cognitive_ext.go`** (CABANG non-frozen).
2. **Tool cognitive baru** → FILE BARU `cognitive_<nama>.go` + init() sendiri (Go gabung init sepaket).
3. **Logic graph inti** (gate/edge/dream/recall) = brain-core FROZEN → lihat `lock/brain.md`, jangan sentuh.

Filosofi owner: file frozen **ga pernah dibuka lagi** — semua evolusi lewat cabang/data/file-baru.
Ini **jalan Flowork BEREVOLUSI** tanpa ngerusak inti stabil.

---

## 0. APA INI

Cognitive Graph (CGM) = **"otak relasi" tiap agent** — bola-bola (node) nyambung kabel (edge),
nyimpen SIAPA/APA + gimana mereka NYAMBUNG (mis. `aola -[goal_is]-> kedaulatan-psikologis`).
Beda dari `brain_search` (cari teks FTS): CGM paham **RELASI antar-entitas** → recall by-makna +
deteksi kontradiksi. Per-agent (twin), di `state.db` (`cognitive_nodes` + `cognitive_edges`).
Arsitektur memori lengkap → `lock/brain.md` §3.

---

## 1. MODEL DATA

**Node** (`cognitive_nodes`): id, label, **type** (person/preference/trait/concept/project/event/
skill/fact/knowledge/doctrine/persona/memory/**instinct**/edu_error/agent/tool), why, where_domain,
confidence, **status** (active/shadow/quarantined), hit_count, embedding.

**Edge** (`cognitive_edges`): from_id, to_id, **relation_type** (kosakata tetap `ValidRelations`:
is_a/part_of/created_by/uses/located_in/causes/prefers/communicates_in_style/goal_is/member_of/
governed_by/about/dst), strength, confidence, **status** (active/shadow/superseded).

**Relasi FUNGSIONAL** (`FunctionalRelations`: is_a, decides_by, located_in, created_by, goal_is,
communicates_in_style) = "satu nilai benar". Kalau ada nilai BARU yang konflik sama yang lama →
**KONTRADIKSI** (tension), lihat §3.

---

## 2. ORPHAN ("bola ga nyambung") — MASALAH LIMIT, BUKAN DATA

**Gejala (owner 2026-06-23):** GUI nampilin ratusan bola **instinct** ngambang ga ada kabel.

**Akar SEBENARNYA (hasil lacak):** di DB instinct **NYAMBUNG semua** (897 instinct, 0 orphan) —
tiap instinct `member_of` hub concept (`brain-root`, `hub-coding-instinct`, `hub-security-instinct`,
`hub-mindset`). TAPI hub itu hit_count RENDAH (=3) + edge `member_of` strength rendah. Viz dulu
cuma load **2000 node** (hit DESC) + **1000 edge** (strength DESC) dari 2241 node / 2324 edge →
hub + edge instinct KE-DROP → instinct keliatan orphan PADAHAL nyambung = **kabel putus PALSU**.

**FIX 1 (switch, cognitive_ext.go):** naikin default load → **node 3000 + edge 6000** (nutup graph
penuh). Hasil: load semua node + edge, **orphan 75% → 10%**. Override: env `FLOWORK_CGM_NODE_LIMIT` /
`FLOWORK_CGM_EDGE_LIMIT`.

**FIX 2 (backfill, 2026-06-23):** sisa 10% (225 genuine orphan: tool 136, agent 34, entity stray 55)
di-link `member_of` ke hub baru `hub-tools`/`hub-agents`/`hub-knowledge` (→ `brain-root`). Hasil:
**orphan 10% → 0%** (graph nyambung TOTAL, 2243 node / 2551 edge). Orphan BARU yang muncul nanti
(graph tumbuh) bisa di-backfill ulang (pola sama) — bisa dijadiin job periodik via cabang kalau perlu.

---

## 3. KONTRADIKSI (tension) + LOOP KLARIFIKASI — "data matang dari obrolan"

**Deteksi** (`cognitive_gate.go` `DetectEdgeContradiction`, dipanggil di `cognitive_dream.go`):
pas relasi fungsional dapet nilai BARU ≠ lama → `RecordTension(old, new)`. New edge **DITAHAN**
(continue), old tetap active. Tension status='open' nunggu **OWNER** mutusin.

**3 LAPIS supaya mr-flow proaktif klarifikasi (owner deside):**
1. **LIHAT** — tool `cognitive_tensions` (read, state:read): daftar kontradiksi {id, subject, relation, old, new}.
2. **SADAR** — tool ke-expose ke mr-flow + persona nyuruh cek pas topik soal fakta/preferensi owner.
3. **RESOLVE** — tool `cognitive_resolve(tension_id, keep='new'|'old')` (write, state:write): keep=new →
   UpsertEdge(new active) + old superseded; keep=old → biarin (old masih active) → lalu ResolveTension.
   ⛔ Persona TEGAS: owner yang decide, mr-flow JANGAN nebak.

**SCHEDULE 3x/hari** (`wakeups` table mr-flow): 3 wakeup self-perpetuating (id `cgm-clarify-1..3`,
+8h/+16h/+24h). Pas fire (`RunDueWakeups` poller, `wakeup_engine.go`): mr-flow cek tensions →
tanya owner 1 klarifikasi via Telegram (`notifyOwnerTelegram`) → re-schedule +8h. Tujuan owner:
**data matang cepat**. Ubah ritme: ganti due_unix wakeup / prompt-nya nyebut interval beda.

**GUI:** tab Cognitive Graph → "⚠ Open contradictions (owner decides)" = list scrollable mandiri
(max-height, ga nyeret halaman) + counter.

---

## 4. TOOLS CGM

| Tool | Cap | Fungsi | File |
|---|---|---|---|
| `graph_recall` | state:read | recall fact-sheet by-makna dari graph | cognitive_tools.go (FROZEN) |
| `cognitive_tensions` | state:read | daftar kontradiksi nunggu owner | cognitive_tensions.go (FROZEN) |
| `cognitive_resolve` | state:write | apply keputusan owner + tutup tension | cognitive_tensions.go (FROZEN) |

API GUI (read-only): `GET /api/agents/cognitive/graph?id=` + `GET /api/agents/cognitive/tensions?id=`.

---

## 5. SWITCH / CABANG (cognitive_ext.go, NON-frozen) — jalan evolusi

Realisasi perintah owner "kasih switch buat kemungkinan filtur tambahan, biar Flowork berevolusi":

| Switch | Default | Env | Guna |
|---|---|---|---|
| `cgmNodeLimit()` | 3000 | `FLOWORK_CGM_NODE_LIMIT` | jumlah node load viz (anti orphan-palsu) |
| `cgmEdgeLimit()` | 6000 | `FLOWORK_CGM_EDGE_LIMIT` | jumlah edge load viz |

Tiap titik switch di file frozen dikasih komentar penunjuk ke `cognitive_ext.go` → AI yang buka
file frozen langsung sadar ada jalan pintas. Filtur CGM masa depan (mode viz, filter, dll) tambah
fungsi/registry di `cognitive_ext.go`, panggil dari titik berkomentar — JANGAN buka file frozen.

---

## 6. PETA FILE & FREEZE

| File | Peran | Freeze |
|---|---|---|
| `internal/agentmgr/cognitive_handlers.go` | read API graph + tensions | **FREEZE** (2026-06-23) |
| `internal/tools/builtins/cognitive_tensions.go` | tool cognitive_tensions + cognitive_resolve | **FREEZE** (2026-06-23) |
| `web/tabs/cognitive.js` | GUI D3 viz + scroll contradictions | **FREEZE** (2026-06-23) |
| `internal/agentmgr/cognitive_ext.go` | CABANG: switch limit + hook | NON-frozen |
| `internal/agentdb/cognitive_graph.go` | node/edge schema + Upsert | FROZEN brain-core (lock/brain.md) |
| `internal/agentdb/cognitive_gate.go` | gate + DetectContradiction + tension | FROZEN brain-core |
| `internal/agentdb/cognitive_dream.go` | digest/projeksi → node/edge + tension | FROZEN brain-core |
| `internal/tools/builtins/cognitive_tools.go` | graph_recall | FROZEN brain-core |
| `cognitive_<nama>.go` (masa depan) | tool CGM baru | NON-frozen (file baru) |

---

## 7. CARA NAMBAH FILTUR (tanpa buka frozen)

- **Tuning limit/perilaku viz** → env switch (cognitive_ext.go §5).
- **Tool CGM baru** → FILE BARU `cognitive_<nama>.go` + init() (akses store via `tools.FromStore(ctx)`,
  panggil method Store yg ADA — ListOpenTensions/UpsertEdge/UpsertNode/ResolveTension/Neighbors/dst).
- **Backfill genuine orphan** → ✅ DONE 2026-06-23 (225 → hub-tools/hub-agents/hub-knowledge, orphan 0%).
  Orphan baru nanti: re-run pola sama (`INSERT member_of orphan→hub`); bisa dijadiin job periodik di cabang.
- **Logic graph inti** (gate/edge/dream) → brain-core FROZEN, butuh izin owner (lock/brain.md).

---

## 8. PANTANGAN

- ❌ Jangan turunin limit node/edge balik ke 500/1000 → orphan-palsu balik.
- ❌ Jangan resolve tension TANPA keputusan owner (data soal owner = owner yang decide).
- ❌ Jangan hapus 3 wakeup `cgm-clarify-*` (itu loop "data matang" 3x/hari).
- ❌ Jangan buka file FROZEN buat filtur baru — pakai cabang (cognitive_ext.go) / file baru / data.
- ❌ Jangan hardcode kosakata relasi di luar `ValidRelations` (UpsertEdge bakal nolak).
