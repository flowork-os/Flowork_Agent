// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B0 brain lokal. E2E verified (add/dedup/FTS-search/get/count
//   + agent pipeline). Schema forward-compat (amplitude/quarantined/confidence
//   buat B1/B5). Extend (constitution/immune methods) → tambah file baru
//   (constitution.go/immune.go) operasi tabel ini, JANGAN modify ini.
//
// brain_drawers.go — Roadmap 2 Fase B0: brain LOKAL per-agent (di state.db).
//
// Tiap warga punya brain SENDIRI — knowledge/experience disimpan lokal +
// dicari pakai FTS5 (BM25), TANPA gantung router. Ini fondasi "self-contained
// > centralized": router mati, agent tetep inget pengalamannya.
//
// Beda peran sama router brain (5jt drawers, korpus shared):
//   - brain lokal (di sini) = PENGALAMAN agent sendiri (kecil, isolated, portable).
//   - router brain          = KORPUS pengetahuan shared (di-query remote via
//                             brain_search_shared, on-demand).
//
// Anti-boros (roadmap 1.5): FTS5 keyword-only (NO embedding di lokal — hemat),
// dedup via content_hash, drawer quarantine buat anti-halu (Fase B5).
//
// Pola di-adapt dari: internal/agentdb/skills_curate.go (ensureCols/Add/List)
// + flowork_Router/internal/brain/{retrieve,write,fts}.go (FTS5 MATCH + bm25).

package agentdb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// BrainDrawer — satu entri knowledge/experience di brain lokal.
type BrainDrawer struct {
	ID          string  `json:"id"`
	Content     string  `json:"content"`
	Wing        string  `json:"wing"`       // kategori besar (general/experience/eureka/constitution/…)
	Room        string  `json:"room"`       // sub-kategori opsional
	MemType     string  `json:"mem_type"`   // experience|eureka|fact|user|constitution|…
	Importance  float64 `json:"importance"` // 0..10 (ranking hint)
	Amplitude   int     `json:"amplitude"`  // sacred always-inject (Fase B1); 999999 = sacred
	ContentHash string  `json:"content_hash"`
	Source      string  `json:"source"`      // siapa nyetor (agent/dream/user/…)
	Quarantined bool    `json:"quarantined"` // Fase B5 immune — ga dipake sampe verified
	Confidence  float64 `json:"confidence"`  // Fase B5 tier-confidence 0..1
	CreatedAt   string  `json:"created_at"`
}

// BrainHit — hasil SearchLocalBrain (drawer + skor relevansi FTS).
type BrainHit struct {
	DrawerID string  `json:"drawer_id"`
	Wing     string  `json:"wing"`
	Room     string  `json:"room"`
	MemType  string  `json:"mem_type"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"` // normalized (0,1], higher = better
}

// Budget anti over-prompt (README_FIRST section 7).
const (
	maxBrainContentBytes = 16 * 1024 // 1 drawer cap 16KB
	defaultLocalBrainK   = 5
	maxLocalBrainK       = 10
	maxBrainSnippetChars = 1000 // truncate tiap hit content
)

// ensureBrainSchema bikin tabel brain lokal (idempotent). Caller WAJIB sudah
// pegang s.mu (dipanggil dari method ber-lock). FTS5 = virtual table, ga ada
// trigger sync — Add nulis ke drawers + fts dua-duanya.
func (s *Store) ensureBrainSchema() {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS brain_drawers (
			id           TEXT PRIMARY KEY,
			content      TEXT NOT NULL,
			wing         TEXT NOT NULL DEFAULT 'general',
			room         TEXT NOT NULL DEFAULT '',
			mem_type     TEXT NOT NULL DEFAULT 'experience',
			importance   REAL NOT NULL DEFAULT 3.0,
			amplitude    INTEGER NOT NULL DEFAULT 0,
			content_hash TEXT NOT NULL DEFAULT '',
			source       TEXT NOT NULL DEFAULT 'agent',
			quarantined  INTEGER NOT NULL DEFAULT 0,
			confidence   REAL NOT NULL DEFAULT 1.0,
			created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at   TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_brain_wing ON brain_drawers(wing)`,
		`CREATE INDEX IF NOT EXISTS idx_brain_amp  ON brain_drawers(amplitude)`,
		`CREATE INDEX IF NOT EXISTS idx_brain_hash ON brain_drawers(content_hash)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS brain_fts USING fts5(
			drawer_id UNINDEXED, content, wing, room, tokenize='porter unicode61'
		)`,
	}
	for _, q := range stmts {
		_, _ = s.db.Exec(q)
	}
}

// AddBrainDrawer simpan 1 drawer ke brain lokal (+ FTS sync). Dedup by
// content_hash: kalau udah ada drawer live dengan content sama → ga insert lagi
// (return id lama, added=false). Pola dari Router AddDrawer.
func (s *Store) AddBrainDrawer(content, wing, room, memType, source string) (id string, added bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureBrainSchema()

	content = strings.TrimSpace(content)
	if content == "" {
		return "", false, fmt.Errorf("content kosong")
	}
	if len(content) > maxBrainContentBytes {
		content = content[:maxBrainContentBytes] + "…[truncated]"
	}
	if wing == "" {
		wing = "general"
	}
	if memType == "" {
		memType = "experience"
	}
	if source == "" {
		source = "agent"
	}

	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	id = hash[:16]

	// Dedup: skip kalau drawer live dengan hash sama udah ada.
	var existing string
	if qerr := s.db.QueryRow(
		`SELECT id FROM brain_drawers WHERE content_hash=? AND deleted_at IS NULL LIMIT 1`, hash,
	).Scan(&existing); qerr == nil && existing != "" {
		return existing, false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err = s.db.Exec(
		`INSERT OR IGNORE INTO brain_drawers
		 (id, content, wing, room, mem_type, content_hash, source, created_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		id, content, wing, room, memType, hash, source, now,
	); err != nil {
		return "", false, fmt.Errorf("insert drawer: %w", err)
	}
	// Sync FTS (no auto-trigger).
	if _, err = s.db.Exec(
		`INSERT INTO brain_fts (drawer_id, content, wing, room) VALUES (?,?,?,?)`,
		id, content, wing, room,
	); err != nil {
		return "", false, fmt.Errorf("insert fts: %w", err)
	}
	return id, true, nil
}

// SearchLocalBrain cari drawer relevan di brain lokal pakai FTS5 BM25.
// Precision-first: coba AND dulu (set kecil, presisi), fallback OR kalau kosong.
// Skip quarantined + deleted. Cap k anti over-prompt.
func (s *Store) SearchLocalBrain(query string, k int) ([]BrainHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureBrainSchema()

	if k <= 0 {
		k = defaultLocalBrainK
	}
	if k > maxLocalBrainK {
		k = maxLocalBrainK
	}
	tokens := ftsTokens(query)
	if len(tokens) == 0 {
		return []BrainHit{}, nil
	}

	hits, err := s.runBrainFTS(joinFTS(tokens, "AND"), k)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 && len(tokens) > 1 {
		hits, err = s.runBrainFTS(joinFTS(tokens, "OR"), k)
		if err != nil {
			return nil, err
		}
	}
	return hits, nil
}

// runBrainFTS — jalanin 1 query FTS5 MATCH, join ke drawers buat metadata +
// filter quarantine/deleted. Caller pegang lock. Pola bm25 dari Router runFTS.
func (s *Store) runBrainFTS(match string, k int) ([]BrainHit, error) {
	rows, err := s.db.Query(
		`SELECT f.drawer_id, d.wing, d.room, d.mem_type, d.content, bm25(brain_fts) AS score
		   FROM brain_fts f
		   JOIN brain_drawers d ON d.id = f.drawer_id
		  WHERE brain_fts MATCH ?
		    AND d.deleted_at IS NULL
		    AND d.quarantined = 0
		  ORDER BY score
		  LIMIT ?`,
		match, k,
	)
	if err != nil {
		return nil, fmt.Errorf("brain fts: %w", err)
	}
	defer rows.Close()

	out := []BrainHit{}
	for rows.Next() {
		var h BrainHit
		var bm25 float64
		if serr := rows.Scan(&h.DrawerID, &h.Wing, &h.Room, &h.MemType, &h.Content, &bm25); serr != nil {
			continue
		}
		if bm25 < 0 {
			bm25 = -bm25
		}
		h.Score = 1.0 / (1.0 + bm25)
		if len(h.Content) > maxBrainSnippetChars {
			h.Content = h.Content[:maxBrainSnippetChars] + "…"
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetBrainDrawer ambil 1 drawer full by id (buat brain_get).
func (s *Store) GetBrainDrawer(id string) (BrainDrawer, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureBrainSchema()

	var d BrainDrawer
	var quar int
	var deletedAt *string
	err := s.db.QueryRow(
		`SELECT id, content, wing, room, mem_type, importance, amplitude,
		        content_hash, source, quarantined, confidence, created_at
		   FROM brain_drawers WHERE id=? AND deleted_at IS NULL`, id,
	).Scan(&d.ID, &d.Content, &d.Wing, &d.Room, &d.MemType, &d.Importance, &d.Amplitude,
		&d.ContentHash, &d.Source, &quar, &d.Confidence, &d.CreatedAt)
	_ = deletedAt
	if err != nil {
		return BrainDrawer{}, false, nil // not found → not an error
	}
	d.Quarantined = quar == 1
	return d, true, nil
}

// CountBrainDrawers — jumlah drawer live (buat status/test).
func (s *Store) CountBrainDrawers() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureBrainSchema()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM brain_drawers WHERE deleted_at IS NULL`).Scan(&n)
	return n, err
}

// ── FTS token helpers (di-adapt dari flowork_Router internal/brain/fts.go) ──

// ftsTokens ubah free-text jadi token FTS5 aman (strip char berbahaya, quote).
func ftsTokens(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var parts []string
	for _, f := range strings.Fields(q) {
		var b strings.Builder
		for _, r := range f {
			switch r {
			case '"', '\'', '?', '.', ',', ':', ';', '!', '(', ')', '[', ']', '{', '}',
				'*', '/', '\\', '|', '&', '#', '@', '+', '=', '<', '>', '`', '~':
				continue
			default:
				b.WriteRune(r)
			}
		}
		clean := b.String()
		if len(clean) < 2 {
			continue
		}
		parts = append(parts, fmt.Sprintf(`"%s"`, clean))
	}
	return parts
}

// joinFTS gabung token ber-quote pakai operator ("AND"/"OR").
func joinFTS(tokens []string, op string) string {
	return strings.Join(tokens, " "+op+" ")
}
