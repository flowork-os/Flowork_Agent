# FLOWORK INSTINCTS — Cara Kerja, Kondisi Sekarang & Roadmap Koloni

> Status: **DRAFT / kondisi-berjalan** (2026-06-25). Owner: Aola Sahidin.
> Catatan: sistem insting koloni BELUM final — doc ini rekam kondisi sekarang + rencana. Pola sama AUTOSLEEP.MD.

---

## 0. CARA KERJA INSTING (inti)

**Insting = pola refleks "WHEN <situasi> → THEN <aksi>"** yang di-recall **by-makna** (semantic), bukan keyword.
- Storage: `cognitive_nodes` `type=instinct` (di agent `state.db`) — field kunci: `label` (teks WHEN→THEN),
  `where_domain` (konteks/peran: universal/bisnis/kehidupan/security/coding/...), `properties` (JSON marker),
  `confidence`, `status=active`, `embedding` (bge-m3 8-bit quantize — WAJIB biar ke-recall).
- Recall: `instinct_recall` → `SearchNodesByEmbedding('instinct', queryEmb, k)` → cosine top-k node `active`.
- **2 TIPE insting (WAJIB jelas):**
  1. **BASIC / PERMANEN (owner)** — ditentuin owner. **= persis 24 insting yang di-upload 2026-06-25.**
     `confidence=1.0`, `properties.tier=basic`, `permanent=true`. **PERMANEN: gak pernah di-hapus,
     di-archive, atau di-demote.** Rangking **PALING ATAS** — kalau ada insting topik-sama, basic SELALU menang.
  2. **AGENT-GROWN (tiap agent)** — **SETIAP agent BISA inject insting sendiri** (dari pengalaman/keputusan),
     `confidence<1.0`, `source=agent_inferred`. Rangking **DI BAWAH** basic. Gak boleh nimpa/nurunin basic.

---

## 0.5. ⭐ INJEKSI PROAKTIF — `maybeInjectInstinct` (LIVE 2026-06-25, FROZEN)

**AKAR yang ditutup (owner: "mobil mewah tapi ga tau naiknya"):** insting DULUNYA cuma **PULL-ONLY**
(agent harus manggil tool `instinct_recall` sendiri) → buat tau dia HARUS recall, agent butuh insting =
**telur-ayam** → kapabilitas parkir, agent **"ga sadar kapan manggil tool/fitur"**. Bandingin: doktrin
(`maybeInjectConstitution`) + antibodi (`maybeInjectAntibodies`) **di-PAKSA masuk** tiap request. Insting nggak.

**Solusi (sekarang JALAN):** router **maksa-inject insting relevan** ke tiap request — sejajar doktrin & antibodi.
Sibling persis `mistakeenrich.go`. Prinsip owner: *"jangan ngarep model manggil sendiri — PAKSA injeksi."*

**Cara kerja (per-request, di dispatcher `:2402`):**
1. `maybeInjectInstinct` (router) ambil `query = pesan user terakhir`.
2. `brain.ListInstinctDrawers` → semua drawer HIDUP `room LIKE 'instinct%'` (no vindex).
3. **Rank `rankInstincts`: `importance × (1 + 2·overlap)`** (token-overlap × importance) — **DETERMINISTIK, NO vindex**
   (jalan walau RI-1 belum). Anti-noise: overlap 0 + importance < 7 → skip. Cap default 3.
4. Inject sbg **system message "augment"** (nempel, gak dominasi persona). **Fails-open** (brain mati/kosong → skip).

**Beda sama recall lama:** ini **PROAKTIF + PUSH** (router, token-overlap, gak butuh agent manggil tool);
`instinct_recall` lama = **PULL** (agent-side, semantic embedding). Dua-duanya hidup, saling lengkap. Begitu
RI-1 (vindex) idup, seleksi bisa di-upgrade ke **semantic** lewat seam `RegisterInstinctSelector` (gak buka freeze).

**SWITCH (Rule 7 — evolusi TANPA buka freeze):**
| Mau | Caranya | Sentuh frozen? |
|---|---|---|
| Matiin / tuning intensitas | ENV `FLOWORK_INSTINCT_INJECT=0` · `FLOWORK_INSTINCT_INJECT_MAX=N` | ❌ |
| **Tumbuh awareness** (nambah insting) | tambah drawer `room=instinct_*` (API `POST /api/brain/drawer` / seed) | ❌ (NOL kode) |
| Ganti seleksi (semantic / scoping #6 / boost domain) | `RegisterInstinctSelector(fn)` di `instinctenrich_ext.go` (NON-frozen) | ❌ |
| Ubah logika inti rank/inject | unfreeze `instinctenrich.go` (sadar + izin owner, CARAFREEZE) | ✅ rare |

**Room baru `instinct_tool` (41 instinct 2026-06-25 = 12 hand + 29 auto-gen dari tool-description):** capability-instinct
WHEN→THEN buat "kapan pakai tool/fitur" — growth-reflex (`ga ada tool → tool_search → tool_create`), recovery
(`error → fallback`), + tiap tool non-core (git/codemap/task/workflow/code_scan/brain_*/dst). *(Scoping #6: agent LUAR
non-flowork nanti SKIP `instinct_tool` via selector-hook — mereka punya tool sendiri, biar gak halu.)*

**⭐ REALISASI TOKEN-CUT (#2C, 2026-06-25):** karena tiap tool punya insting (ke-recall) + injeksi proaktif, **`maxExposedTools`
bisa diturunin drastis**: `tool_specs.go` jadi ENV-switch `FLOWORK_MAX_EXPOSED_TOOLS` (default 56, owner-lokal **16 core-only**).
Hasil ukur: **56→16 tool, ~10.7k→~2.0k byte schema (~8.7k tok hemat/turn)**. Tool yg di-drop dari expose TETAP ke-pakai:
insting-tool kasih tau namanya → agent `tool_search(keyword)` (terverifikasi nemu git/codemap/task/workflow) → call. Revert
instan: set ENV balik 56. Inilah "tool-as-instinct" yg bikin token biang (~55% prompt) anjlok TANPA ilangin kapabilitas.

**Bukti live:** "kirim notif ke owner"→`[tool tool tool]` · "berita crypto dari internet"→`[tool tool bisnis]` ·
"ga punya tool convert pdf"→growth-reflex. 6 unit test ijo · 0 regresi (router+brain+TestKernelFreeze).

**File (peta — lihat juga §5):**
- `router/internal/router/instinctenrich.go` — **🔒 FROZEN** (chattr+i + hash KERNEL_FREEZE.md): `maybeInjectInstinct` + `rankInstincts` + `buildInstinctSystem` + `RegisterInstinctSelector`.
- `router/internal/brain/instincts.go` — **🔒 FROZEN**: `ListInstinctDrawers` (query drawer room=instinct_*).
- `router/internal/router/instinctenrich_ext.go` — **✏️ NON-frozen growth-point** (daftar selector custom).
- `router/internal/router/dispatcher.go` + `dispatcher_stream.go` — hook 1-baris `maybeInjectInstinct(...)` = **soft-lock** (NON-chattr, host evolve; persis pola autosleep).

---

## 1. KONDISI SEKARANG (2026-06-25) — jujur, SETENGAH JADI

**Yang JALAN (agent Mr.Flow, lokal `agent/agents/mr-flow/workspace/state.db`):**
- **24 insting BASIC** — conf 1.0, marker `basic/permanent`, ber-embedding, per-domain
  (universal 9 · bisnis 7 · kehidupan 4 · security 4). **Recall KEBUKTI** (chat bisnis → insting "platform-massa/anti-mainstream" nongol).
- **26 drawer BIOGRAFI** literal (verbatim + timestamp) — nama/biografi/waktu Aola, room=keramat. FTS jalan.

**✅ UPDATE 2026-06-25:** insting router (room `instinct_*`, sekarang **36**: 24 owner + 12 tool) udah **DIPAKAI
LIVE** lewat injeksi proaktif **§0.5** (`maybeInjectInstinct`, token-overlap, gak butuh vindex). Jadi "kapan
manggil tool/fitur" UDAH ketutup. Yang di bawah ini sisa-PR layer koloni (display GUI, scoping, projeksi).

**✅ FIXED 2026-06-25 — GUI Brain→Instincts nampil 36 insting (owner: "list ga muncul, analisa dulu"):**
36 insting (room `instinct_*`) JALAN buat injeksi (§0.5) TAPI dulu **"No instincts found"** di halaman GUI.
**Diagnosa LAMA di doc ini SALAH** (nyalahin `SemanticRetrieve`/`vindex`) — akar ASLI (dibuktiin live):
query browse GUI `/api/brain/wing?wing=training_data&room_like=%instinct` meleset DUA-duanya — (a)
`%instinct` = SQL **ENDS-with**, room-nya `instinct_*` = **STARTS-with** → 0; (b) `wing=training_data`,
padahal insting wing-nya `doctrine`(24)+`capability`(12). **FIX (cabut-akar, no migrasi data, no break
injeksi):** endpoint BARU **`GET /api/brain/instincts`** (`handlers_brain_instincts.go`, NON-frozen) pakai
**sumber yang SAMA dgn injeksi** = `brain.ListInstinctDrawers` (`room LIKE 'instinct%'` lintas-wing) → GUI
`loadBrainInstincts` panggil itu. Jadi yang KE-LIHAT = persis yang KE-INJECT. Verified live: endpoint balik
36 (instinct_tool 12 + bisnis 7 + kehidupan 4 + security 4 + universal 9), pager OK, served HTML kebawa.
⚠️ **JANGAN** migrasi insting ke room `flowork_instinct` (kanonik lama §2) — injeksi §0.5 query `instinct%`
(starts-with) → pindah room = injeksi PUTUS. `instinct_*` = konvensi yang dipertahanin.

**Yang BELUM beres (router / shared):**
- **Penyebab + KESALAHAN (catat biar gak diulang):**
  1. ⛔ `brain.vindex` (854MB) ke-HAPUS pas bersih-bersih disk → **index semantik router rusak** →
     `SemanticRetrieve` jalan setengah (FTS-fallback) → drawer baru gak ke-index. **vindex BUKAN sampah.**
  2. ⛔ Injeksi ke router dilakuin via **SQL langsung + room `instinct_<domain>`** (NGAKAL) — bukan jalur
     resmi. Room kanonik halaman = **`flowork_instinct`**, bukan `instinct_<domain>`.
  3. ⚠️ Biografi (personal) ke-promote ke router room `knowledge` lewat federation share-job → **isu privasi D8**
     (personal harusnya LOKAL). Perlu dibalikin.
  4. Belum ada **scoping per-peran** — semua insting ke-recall, belum di-gate by `where_domain`.

→ Ringkas: **insting basic untuk 1 agent (Mr.Flow) UDAH jalan. Layer koloni (shared+scoped+index) belum.**

---

## 2. CARA INPUT INSTING YANG BENER (kanonik — JANGAN ngakal SQL lagi)

| Cara | Endpoint / jalur | Buat apa |
|---|---|---|
| **GUI `+ Add Instinct`** (owner) | `POST /api/brain/drawer` body `{content, wing:"training_data", room:"flowork_instinct", memType:"project"}` | owner tambah 1 insting BASIC/permanen manual |
| **GUI `Run Dream Mode`** | `GET /api/brain/wing?wing=cognitive_graph_dream` | digest memori/chat → insting/graph (otomatis dari pengalaman) |
| **TOOL `instinct_add`** (tiap agent) — **BELUM ADA, dibangun RI-3** | agent panggil tool → `EmbedText`→`Quantize`→`UpsertNode(type=instinct, where_domain=peran, conf<1.0, source=agent_inferred)` | **SETIAP agent inject insting sendiri** (agent-grown) |
| **Projeksi router→agent** | scratch `_scratch_cgm/instproj` (baca drawer router room insting → UpsertNode agent + embedding) | sebar insting shared ke graph agent biar ke-recall |
| **Seed batch (owner)** | scratch `_scratch_cgm/addinstinct` (EmbedText→Quantize→UpsertNode) | seed banyak insting BASIC sekaligus |

**Aturan input:** teks **WHEN→THEN**, **GENERIC** (NOL data personal/nama — biografi pisah ke memori keramat),
tag `where_domain` = konteks/peran. Owner-basic → `confidence=1.0` + `properties.tier=basic`.
⛔ **JANGAN:** insert SQL langsung tanpa embedding · room asal-asalan · hapus `brain.vindex`.

---

## 3. ARSITEKTUR TARGET — KOLONI BERLAPIS (1000 semut)

```
[L0] UNIVERSAL : insting-universal (5W1H/anti-mustahil/anti-halu/error-edukasi) → SEMUA agent → cache sekoloni
[L1] ROLE      : insting-peran (coder/hacker/bisnis/kehidupan...) → per-arketipe → di-scope where_domain
[L2] AGENT     : insting spesifik + agent-grown (tipe-2) → per-semut
[L3] DINAMIS   : insting di-recall by-makna, di-scope per-peran, cuma yang relevan ke-inject
```
**Tiap agent insting SENDIRI sesuai tugas** (coder ≠ hacker ≠ bisnis; Mr.Flow = kehidupan/bisnis/marketing)
= filter `where_domain` sesuai peran agent.

**GATE "muncul cuma saat dibutuhkan" (2-level, ganti hardcode-konstitusi):**
1. **ROLE** (`where_domain`): agent cuma narik insting domain-nya → muncul di **PERAN yang tepat**.
2. **TASK** (semantic relevance, udah ada): dalam scope, embedding-recall cuma narik yg mirip query →
   muncul di **TUGAS yang tepat**.
Cuma yang lolos **ROLE × TASK** ke-inject. Universal-sejati (anti-halu/5W1H) boleh di konstitusi (selalu on).

**Privasi:** insting generic → **shared router** (semua agent inherit). Biografi/personal → **LOKAL agent** (D8).

---

## 4. ROADMAP INSTING KOLONI (urut — JELAS, anti multi-tafsir)

> Tiap step format SAMA: **Tujuan · Langkah · SELESAI-kalau (uji konkret) · Status**. Dikerjain BERURUTAN.

**RI-1 — PULIHIN INDEX SEMANTIK**
- Tujuan: halaman Brain→Instincts nampil + semantic recall router pulih.
- Langkah: `cd router` → `go run ./cmd/brain-reembed` (embed ulang drawer) → `go run ./cmd/brain-buildindex` (bikin `brain.vindex`) → restart router.
- SELESAI kalau: `GET /api/brain/search-drawers?query=gengsi` balikin insting DAN halaman nampil (bukan "No instincts found").
- Status: **BELUM** (vindex ke-hapus pas bersih disk).

**RI-2 — RE-INPUT 24 BASIC VIA JALUR RESMI**
- Tujuan: 24 insting basic masuk router cara kanonik (auto embed+index), buang drawer SQL-ngakal.
- Langkah: hapus drawer room `instinct_*` (SQL lama) → re-add 24 via `POST /api/brain/drawer {content, wing:"training_data", room:"flowork_instinct", memType:"project"}`.
- SELESAI kalau: 24 insting di room `flowork_instinct`, ber-embedding, ke-search + nampil di halaman.
- Status: **BELUM**.

**RI-3 — TOOL `instinct_add` (SETIAP AGENT BISA INJECT)** ← *requirement owner*
- Tujuan: tiap agent inject insting SENDIRI (agent-grown), bukan cuma owner.
- Langkah: bikin tool BARU `instinct_add` (file non-frozen) → `EmbedText`→`Quantize`→`UpsertNode(type=instinct, where_domain=peran-agent, confidence<1.0, source=agent_inferred, properties.tier=grown)`. Daftar ke tool registry semua agent.
- SELESAI kalau: agent panggil `instinct_add("WHEN... -> ...")` → node insting baru ADA + ke-recall via `instinct_recall`, conf < basic.
- Status: **BELUM** (`instinct_recall` skrg read-only).

**RI-4 — GATE PERMANEN (basic SELALU di atas, gak bisa diutak agent)**
- Tujuan: 24 insting owner PERMANEN; agent-grown gak bisa nimpa/nurunin/ngehapus.
- Langkah: basic `confidence=1.0`+`tier=basic`+`permanent=true`; di ranking recall, basic di-urut DULUAN; exclude basic dari archive/demote; `instinct_add` (RI-3) DILARANG nulis ke node basic.
- SELESAI kalau: insting basic gak pernah ke-archive/demote + selalu muncul di atas grown topik-sama.
- Status: **SEBAGIAN** (conf 1.0 + marker udah; jaminan-urut + proteksi-tulis belum).

**RI-5 — SCOPING PER-PERAN (role × task — "muncul cuma saat dibutuhkan")**
- Tujuan: insting muncul cuma di peran + tugas yang tepat (ganti hardcode-konstitusi, hemat token).
- Langkah: gate `where_domain` di `instinct_recall` — filter domain-peran agent DULU, baru semantic-relevance.
- SELESAI kalau: agent coding query "refactor" → 0 insting bisnis; Mr.Flow query bisnis → insting bisnis nongol.
- Status: **BELUM** (`instinct_recall.go` FROZEN → izin unfreeze, Rule 1).

**RI-6 — SHARED + PROJEKSI (1000 semut inherit dari 1 sumber)**
- Tujuan: insting basic di router → tiap agent inherit otomatis (bukan duplikat 24×1000).
- Langkah: `instproj` baca room `flowork_instinct` router → project ke `cognitive_nodes` agent scoped per-peran + embedding.
- SELESAI kalau: agent BARU otomatis punya insting universal + insting-peran-nya, tanpa re-seed manual.
- Status: **BELUM**.

**RI-7 — PRIVASI (biografi lokal-only)**
- Tujuan: data personal Aola gak di shared-router (D8).
- Langkah: cabut biografi dari router room `knowledge` → balikin lokal agent; perkuat gate federation share.
- SELESAI kalau: 0 drawer personal/biografi di router brain; biografi cuma di agent `state.db`.
- Status: **BELUM** (ke-promote ke router lewat federation share-job).

> Aturan kerja: tiap step → eksperimen → ukur → uji stabil → **ACC owner** → baru push. Gagal → rollback dari GitHub (lihat `opus_roadmap.md`).

---

## 5. FILE YANG DILEWATI (peta)

**Agent (storage + recall + embed):**
- `internal/agentdb/cognitive_graph.go` — `CogNode`/`UpsertNode` (storage insting).
- `internal/agentdb/cognitive_recall.go` — `SearchNodesByEmbedding` (recall semantic).
- `internal/agentdb/cognitive_resolve.go` — `Quantize` (embedding 8-bit).
- `internal/tools/builtins/instinct_recall.go` — tool recall insting (FROZEN).
- `internal/routerclient/embed.go` — `EmbedText` (bge-m3 via router).
- `_scratch_cgm/addinstinct` · `_scratch_cgm/instproj` — seed + projeksi (scratch, gitignored).

**Router (shared + halaman):**
- `handlers_brain_views.go` — `brainAddDrawerHandler` (`/api/brain/drawer`) + `brainSearchDrawersHandler` (`/api/brain/search-drawers`).
- `internal/brain/write.go` — `brain.AddDrawer` (insert drawer + FTS).
- `internal/brain/semantic.go` — `SemanticRetrieve` / `vectorRetrieve` (vector + FTS-fallback).
- `cmd/brain-reembed` · `cmd/brain-buildindex` — embed ulang + bangun `brain.vindex` (LOCKED soft).
- `router/web/static/index.html` — halaman Brain→Instincts (`loadBrainInstincts`/`openInstinctAdd`/`triggerDreamCycle`).

**Data:** `agent/.../state.db` (cognitive_nodes, brain_drawers) · `router/brain/flowork-brain.sqlite` (drawers room `flowork_instinct`, vindex).
