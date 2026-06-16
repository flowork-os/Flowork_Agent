// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-16 (LOCKED ≠ FREEZE).
// AI lain: JANGAN otak-atik tanpa izin owner. Teruji (TestSemanticLive: vector murni by-makna).
//
// semantic.go — #6 ARSITEK BARU: pencarian brain by-MAKNA (Quantum Recall, vector murni).
//
// TUJUAN (buat AI lain): agent cari memori by-MAKNA (semantic), BUKAN keyword. Owner 2026-06-16:
// "fokus arsitek baru saja, jangan hybrid" — JADI INI BUKAN fusion FTS+vector (yg bikin bingung).
// SATU JALUR: kalau index semantik (brain.vindex, dari brain-buildindex atas 5jt drawer) udah ada
// -> pakai VECTOR murni. Kalau belum ada (re-embed masih jalan ~jam-an) -> fallback FTS SEMENTARA
// biar brain gak mati pas transisi. Begitu index jadi, otomatis full semantic.
//
// Query di-embed bge-m3 LOKAL (provider 'local' — engine SAMA dgn index -> vektor align). Konten
// hit diambil dari tabel `drawers` (PK lookup) di brain 5jt yg sama. retrieve.go (LOCKED, FTS)
// gak disentuh — fallback manggil Retrieve apa adanya.
package brain

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/brain/vecindex"
	"github.com/flowork-os/flowork_Router/internal/providers/embedding"
)

const vindexEnv = "FLOWORK_BRAIN_VINDEX"

var (
	vMu  sync.Mutex
	vIdx *vecindex.Index
)

// vindexPath — lokasi index: env FLOWORK_BRAIN_VINDEX > <exe_dir>/brain/brain.vindex > cwd. PORTABLE.
func vindexPath() string {
	if p := strings.TrimSpace(os.Getenv(vindexEnv)); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "brain", "brain.vindex")
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	return filepath.Join("brain", "brain.vindex")
}

// loadVIndex — load index sekali (cache). nil kalau file belum ada (vector OFF -> FTS sementara).
// Di-retry tiap call sampai file muncul (index baru jadi setelah re-embed selesai).
func loadVIndex() *vecindex.Index {
	vMu.Lock()
	defer vMu.Unlock()
	if vIdx != nil {
		return vIdx
	}
	p := vindexPath()
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	idx, err := vecindex.Load(p)
	if err != nil {
		return nil
	}
	vIdx = idx
	return vIdx
}

// VectorReady — true kalau index semantik udah ke-load (buat /status & debug).
func VectorReady() bool { return loadVIndex() != nil }

// embedQueryLocal — embed query pakai bge-m3 LOKAL (provider 'local'), balik unit []float32.
func embedQueryLocal(ctx context.Context, query string) ([]float32, error) {
	p := embedding.Get("local")
	if p == nil {
		return nil, fmt.Errorf("embedder 'local' belum ke-register")
	}
	res, err := p.Embed(ctx, embedding.Request{Input: []string{query}})
	if err != nil {
		return nil, err
	}
	if len(res.Data) == 0 || len(res.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding kosong")
	}
	raw := res.Data[0].Embedding
	v := make([]float32, len(raw))
	var ss float64
	for i, x := range raw {
		v[i] = float32(x)
		ss += x * x
	}
	if n := float32(math.Sqrt(ss)); n > 0 {
		for i := range v {
			v[i] /= n
		}
	}
	return v, nil
}

// vectorRetrieve — semantic murni: embed -> vecindex top-k -> konten dari drawers. nil kalau off.
func vectorRetrieve(ctx context.Context, db *sql.DB, query string, limit, maxLen int) []Snippet {
	idx := loadVIndex()
	if idx == nil || db == nil {
		return nil
	}
	qv, err := embedQueryLocal(ctx, query)
	if err != nil || len(qv) != idx.Dim() {
		return nil
	}
	hits := idx.Search(qv, limit)
	if len(hits) == 0 {
		return nil
	}
	ph := make([]string, len(hits))
	args := make([]any, len(hits))
	for i, h := range hits {
		ph[i] = "?"
		args[i] = h.ID
	}
	rows, err := db.QueryContext(ctx,
		"SELECT id, wing, room, content FROM drawers WHERE id IN ("+strings.Join(ph, ",")+")", args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	byID := map[string]Snippet{}
	for rows.Next() {
		var id, wing, room, content string
		if rows.Scan(&id, &wing, &room, &content) == nil {
			if maxLen > 0 {
				content = truncateRunes(content, maxLen)
			}
			byID[id] = Snippet{DrawerID: id, Wing: wing, Room: room, Content: content}
		}
	}
	// jaga URUTAN vector (relevansi makna), skor = dot ter-normalisasi (0,1].
	out := make([]Snippet, 0, len(hits))
	var top float64 = 1
	if len(hits) > 0 && hits[0].Score > 0 {
		top = float64(hits[0].Score)
	}
	for _, h := range hits {
		if sn, ok := byID[h.ID]; ok {
			sn.Score = float64(h.Score) / top // 1.0 buat teratas, turun (relatif)
			out = append(out, sn)
		}
	}
	return out
}

// SemanticRetrieve — ARSITEK BARU (by-makna). Vector murni kalau index siap; FTS SEMENTARA kalau
// belum (transisi). BUKAN hybrid/fusion. Drop-in pengganti Retrieve di handler search-drawers.
func SemanticRetrieve(ctx context.Context, db *sql.DB, query string, opts RetrieveOpts) ([]Snippet, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 6
	}
	if vec := vectorRetrieve(ctx, db, query, limit, opts.MaxContentLen); len(vec) > 0 {
		return vec, nil // semantic murni (arsitek baru)
	}
	return Retrieve(ctx, db, query, opts) // fallback SEMENTARA (index belum jadi)
}
