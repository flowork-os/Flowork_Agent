# 🗺️ Code Map — Dokumentasi GUI

---

## Overview

Code Map adalah **sistem auto-dokumentasi dan dependency graph** FloworkOS — sebuah "peta planet" yang memvisualisasikan semua file Go dan JS dalam proyek beserta hubungan dependency di antara mereka.

Ini bukan sekadar tree view file biasa. Code Map memiliki kapabilitas:
1. **Visualisasi planet** — setiap file adalah "planet", edge = dependency (import)
2. **Health Score** — setiap file punya skor kesehatan 0.0-1.0 berdasarkan: ada tests, ada docs, line count, issues
3. **Impact Analysis** — "kalau file X berubah, file apa yang terdampak?" (transitive BFS)
4. **Zombie Detection** — file yang tidak di-import siapapun dan tidak mengimport siapapun (dead code candidates)
5. **Auto-documentation** — markdown docs per-file yang di-generate dari doc comments
6. **Re-index** — trigger scanning ulang seluruh codebase ke database

**Kegunaan praktis:**
- Sebelum refactor besar: cek impact analysis dulu, tahu apa yang mungkin break
- Code review: lihat health score file yang diubah
- Dead code cleanup: gunakan zombie detector (dengan hati-hati, ~LOW confidence banyak false positive)
- Onboarding: Gemini/warga baru bisa navigate kodebase tanpa perlu `grep` manual

---

## File yang Terlibat

### Backend (Go)
| File | Fungsi |
|---|---|
| [internal/guiapi/codemap.go](../../internal/guiapi/codemap.go) | 9 endpoint handler Code Map |
| [internal/codeindex/](../../internal/codeindex/) | Core indexer: `Indexer`, `IndexAll()`, `IndexFile()` |
| [brain/db/shared.go](../../brain/db/shared.go) | `Shared()` — codemap pakai brain SQLite |

### Frontend (JS)
| File | Fungsi |
|---|---|
| [internal/guiapi/static/js/app.js](../../internal/guiapi/static/js/app.js) | Tab router — load `/tabs/codemap.js` |
| `/tabs/codemap.js` | Panel renderer dengan visualisasi planet graph |

### Database
| DB | Tabel yang Diakses |
|---|---|
| `flowork-brain.sqlite` | `codemap_nodes` (semua file info) |
| `flowork-brain.sqlite` | `codemap_edges` (dependency graph) |
| `docs/auto/<path>.md` | Auto-generated markdown docs per file |

### Schema Tabel
```sql
-- codemap_nodes
CREATE TABLE codemap_nodes (
  path TEXT PRIMARY KEY,
  name TEXT,
  pkg TEXT,
  file_type TEXT,       -- 'go', 'js', 'md', etc
  line_count INT,
  size_bytes INT,
  exported_symbols TEXT, -- JSON array
  doc_comment TEXT,
  health_score REAL,     -- 0.0 - 1.0
  has_tests INT,         -- 0 atau 1
  has_docs INT,          -- 0 atau 1
  issues TEXT,           -- JSON array of issue strings
  last_indexed TEXT      -- RFC3339 timestamp
);

-- codemap_edges
CREATE TABLE codemap_edges (
  from_path TEXT,  -- file yang mengimport
  to_path TEXT,    -- file yang diimport
  edge_type TEXT   -- 'import', 'include', etc
);
```

---

## Sub-menu / Tab

### Tab 1: Planet Graph Visualization
Visualisasi interaktif semua file sebagai "planet" dengan edges menunjukkan dependency.

**Interaksi:**
- Klik satu planet → expand view (lihat semua neighbors langsung)
- Hover planet → tooltip dengan metadata (health score, line count, package)
- Zoom + pan dengan scroll dan drag
- Color coding berdasarkan health score (hijau = sehat, merah = bermasalah)

### Tab 2: Health Report
Tabel semua file diurutkan dari health score terendah (paling bermasalah) ke tertinggi.

**Kolom per file:**
- Path, Name, File Type
- Health Score (0.0-1.0)
- Line Count
- Has Tests (✅/❌)
- Has Docs (✅/❌)
- Issues (daftar masalah yang terdeteksi)

### Tab 3: Impact Analysis
Tool untuk tanya: "Kalau file X saya ubah, apa yang terdampak?"

**Input:** Path file
**Output:** List file yang transitively dependent (BFS sampai depth 5)
- Degree 1 = direct dependent
- Degree 2 = indirect (dependent dari dependent)
- dst sampai degree 5

### Tab 4: Zombie Detector
File yang tidak di-import siapapun DAN tidak mengimport siapapun.

**Per zombie:**
- Path, line count, exported symbols count
- Sibling count (file lain di direktori sama)
- **Confidence**: HIGH / MEDIUM / LOW
- Notes (kenapa mungkin false positive)

### Tab 5: Auto-Docs Viewer
Baca auto-generated markdown documentation untuk satu file.

**Sumber:** File di `docs/auto/<path>.md` yang di-generate saat reindex.
**Fallback:** Kalau file tidak ada, baca `doc_comment` dari DB.

### Tab 6: Reindex Control
Trigger scanning ulang codebase.

**Mode:**
- **Full Reindex** — scan seluruh workspace (async, bisa 1-5 menit untuk codebase besar)
- **Partial Reindex** — re-index satu file spesifik (sync, instant)

---

## API Endpoints

### `GET /api/codemap/graph`
**Fungsi:** Ambil semua nodes + edges untuk visualisasi planet.
**Response:**
```json
{
  "nodes": [...],
  "edges": [
    {"from": "internal/core/agent.go", "to": "internal/provider/client.go", "edge_type": "import"},
    ...
  ],
  "node_count": 450,
  "edge_count": 1200,
  "generated_at": "2026-04-26T10:00:00Z"
}
```

### `GET /api/codemap/node?path=internal/core/agent.go`
**Fungsi:** Detail satu file (node) + daftar deps dan dependents.
**Response:**
```json
{
  "node": {
    "path": "internal/core/agent.go",
    "name": "agent.go",
    "pkg": "core",
    "file_type": "go",
    "line_count": 450,
    "size_bytes": 18500,
    "exported_symbols": ["Agent", "NewAgent", "Run"],
    "doc_comment": "Package core implements the main agent loop...",
    "health_score": 0.75,
    "has_tests": true,
    "has_docs": true,
    "issues": ["no inline comments on exported methods"],
    "last_indexed": "2026-04-26T09:00:00Z",
    "dependency_count": 8,
    "dependent_count": 12
  },
  "deps": ["internal/provider/client.go", "internal/tools/registry.go", ...],
  "dependents": ["cmd/flowork/main.go", "cmd/flowork-watcher/main.go", ...]
}
```

### `GET /api/codemap/impact?path=internal/tools/registry.go`
**Fungsi:** Transitive impact analysis (siapa yang terdampak kalau file ini berubah).
**Response:**
```json
{
  "source": "internal/tools/registry.go",
  "impact": [
    {"path": "internal/core/agent.go", "degree": 1},
    {"path": "cmd/flowork/main.go", "degree": 2},
    {"path": "cmd/flowork-watcher/main.go", "degree": 2}
  ],
  "total_impacted": 3
}
```
**Note:** BFS depth max 5. Lebih dari 5 hop = tidak di-hitung (terlalu jauh untuk jadi concern langsung).

### `GET /api/codemap/health`
**Fungsi:** Health report semua file, sorted dari skor terendah.
**Response:**
```json
{
  "files": [
    {"path": "...", "health_score": 0.2, "issues": ["no tests", "no doc comment"], ...},
    ...
  ],
  "total_files": 450,
  "avg_health": 0.68,
  "generated_at": "..."
}
```

### `GET /api/codemap/docs?path=internal/core/agent.go`
**Fungsi:** Baca auto-generated docs untuk satu file.
**Sumber:** `docs/auto/<path>.md` → fallback ke `doc_comment` di DB.

### `POST /api/codemap/reindex`
**Fungsi:** Trigger reindex. Full atau partial.
**Params:** `?file=path` untuk partial. Tanpa param = full (async).
**Response Full (async):**
```json
{"ok": true, "message": "reindex started"}
```
**Response Partial (sync):**
```json
{"ok": true, "file": "internal/core/agent.go"}
```
**Response kalau sudah running:**
```json
{"ok": false, "message": "already running"}  // HTTP 409
```

### `GET /api/codemap/status`
**Fungsi:** Status indexer saat ini.
**Response:**
```json
{
  "running": false,
  "last_reindex": "2026-04-26T09:00:00Z",
  "last_stats": {
    "files_indexed": 450,
    "edges_created": 1200,
    "errors": 3,
    "duration_ms": 45000
  },
  "node_count": 450,
  "edge_count": 1200
}
```

### `GET /api/codemap/roots`
**Fungsi:** Entry-point files — file yang tidak di-import siapapun (dependent_count = 0).
**Kegunaan:** Titik awal navigasi graf — biasanya file `main.go` dan entry scripts.

### `GET /api/codemap/expand?path=...`
**Fungsi:** Satu node + semua direct neighbors beserta edges. Dipakai saat user klik planet di visualisasi.
**Kegunaan:** "Expand satu level" dari node yang diklik tanpa load seluruh graph.

### `GET /api/codemap/zombies`
**Fungsi:** File zombie dengan confidence scoring.
**Response:**
```json
{
  "zombies": [
    {
      "path": "internal/dreamstate/old_entry.go",
      "line_count": 45,
      "exported_symbols": 0,
      "sibling_count": 0,
      "confidence": "HIGH",
      "notes": []
    },
    {
      "path": "internal/agents/types.go",
      "exported_symbols": 8,
      "sibling_count": 5,
      "confidence": "LOW",
      "notes": [
        "Punya 8 simbol yang di-export — kemungkinan dipakai package lain tapi resolver gagal",
        "Ada 5 file lain di paket yang sama"
      ]
    }
  ],
  "count": 2,
  "high_confidence": 1,
  "warning": "Zombie dengan confidence LOW/MEDIUM kemungkinan false positive. Verifikasi manual sebelum hapus."
}
```

---

## Setiap Tombol & Fungsinya

| Tombol | API yang Dipanggil | Efek |
|---|---|---|
| 🔄 Refresh Graph | `GET /api/codemap/graph` | Reload seluruh visualisasi |
| Klik planet (node) | `GET /api/codemap/expand?path=...` | Expand neighbors langsung |
| 🎯 Impact Analysis | `GET /api/codemap/impact?path=...` | Analisis transitive dependents |
| 💚 Health Report | `GET /api/codemap/health` | Buka tabel health semua file |
| 🧟 Zombie Detector | `GET /api/codemap/zombies` | List dead code candidates |
| 📄 View Docs | `GET /api/codemap/docs?path=...` | Baca auto-generated markdown |
| 🔍 Node Detail | `GET /api/codemap/node?path=...` | Detail metadata satu file |
| 🚀 Full Reindex | `POST /api/codemap/reindex` | Scan ulang seluruh codebase (async) |
| ⚡ Reindex This File | `POST /api/codemap/reindex?file=...` | Re-index satu file (sync) |
| 📊 Status | `GET /api/codemap/status` | Cek apakah indexer sedang jalan |
| 🌱 View Roots | `GET /api/codemap/roots` | Lihat entry-point files |

---

## Logika Bisnis

### Health Score Calculation (di `internal/codeindex/`)
```
health_score = 1.0 (base)
  - 0.3 kalau tidak ada tests (has_tests = false)
  - 0.2 kalau tidak ada doc comment (doc_comment empty)
  - 0.1 per issue yang terdeteksi (capped agar tidak negative)
  - Bonus +0.1 kalau line_count <= 200 (modular)
```

### Zombie Confidence Logic
```go
isSpecialName := name ∈ {"main.go", "init.go", "doc.go", "errors.go", "types.go", "constants.go"}

if sibling_count > 0 || exported_symbols > 5 || isSpecialName {
    confidence = "LOW"    // Kemungkinan false positive
} else if exported_symbols == 0 && sibling_count == 0 && line_count < 50 {
    confidence = "HIGH"   // Kemungkinan dead code sejati
} else {
    confidence = "MEDIUM"
}
```

### Impact BFS (max depth 5)
```
queue = [source_file]
visited = {source_file}

for degree = 1 to 5:
    for each file in queue:
        find all FROM files WHERE to_path = file  // who imports this?
        add unvisited ones to impact list with current degree
        advance queue to next level
```

Kenapa max 5? Lebih dari 5 hop dependency = terlalu jauh untuk jadi concern langsung. Dan untuk codebase besar (450+ file), BFS unlimited bisa return ratusan file tidak relevan.

### Singleton Indexer
```go
var codemapIndexer *codeindex.Indexer  // singleton

func getOrCreateIndexer(ws string) (*codeindex.Indexer, error) {
    codemapMu.Lock()
    defer codemapMu.Unlock()
    if codemapIndexer != nil {
        return codemapIndexer, nil  // reuse
    }
    // create new
}
```

Satu indexer per GUI process. Thread-safe via `codemapMu` mutex.

### 409 Conflict pada Full Reindex
```go
if ix.IsRunning() {
    return HTTP 409 {"ok": false, "message": "already running"}
}
```
Tidak bisa spawn 2 full reindex sekaligus (race di DB write).

---

## Edge Case & Error State

| Kondisi | Behavior |
|---|---|
| Reindex sedang jalan + user klik Reindex lagi | HTTP 409, pesan "already running", tombol disabled di UI |
| `codemap_nodes` tabel kosong (belum pernah reindex) | Graph kosong, health report kosong — tampilkan hint "Run Reindex" |
| File path traversal di `?path=` | Tidak ada guard eksplisit di handler, tapi codeindex.Indexer tidak expose raw filesystem |
| Zombie file = `main.go` | Confidence = LOW, ada note "File main.go — entry point, wajar tidak di-import" |
| `docs/auto/<path>.md` tidak ada | Fallback ke `doc_comment` dari DB → kalau keduanya kosong, HTTP 404 "run reindex first" |

---

## Catatan Teknis

1. **Reindex bisa lambat** — Full reindex untuk 450+ file Go bisa makan 30-60 detik. Jangan interrupt. Status endpoint bisa di-poll untuk progress.

2. **Brain DB (bukan settings)** — `codemap_nodes` dan `codemap_edges` ada di `flowork-brain.sqlite`. Bisa di-reset bersama brain reset tanpa kehilangan operational data.

3. **JS files tracker terbatas** — Parser codeindex hanya track `relative imports` di JS (bukan npm imports). Jadi zombie detection untuk file JS kemungkinan banyak false positive (files yang pakai npm packages tidak akan punya outgoing edges tracked).

4. **Auto-docs di `docs/auto/`** — Path separator di-convert ke `__` untuk nama file: `internal/core/agent.go` → `docs/auto/internal__core__agent.go.md`. Direktori `docs/auto/` bisa sangat besar (900+ file) — tidak di-commit ke git biasanya.

5. **`lastStats` hanya dari last full reindex** — Partial reindex tidak update `lastStats`. Untuk monitoring, full reindex yang paling informatif.

6. **Zombie != Delete** — Temuan zombie dengan confidence HIGH pun perlu manual verification. Scanner false positive rate untuk zombie detection lebih tinggi dari 88% karena parser tidak track intra-package references dan interface implementations.

7. **Protected Core di Impact Analysis** — Kalau impact analysis untuk `internal/core/agent.go` atau `internal/tools/registry.go` (Protected Core Tier 1), hasilnya panjang (banyak yang terdampak). Ini konfirmasi kenapa perubahan ke Protected Core butuh BFT 2-of-3 quorum.
