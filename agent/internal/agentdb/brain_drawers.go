// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN brain-core — jangan edit tanpa unfreeze owner. Arsitektur & alasan: lihat lock/brain.md

package agentdb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// MemTypeClassifierHook adalah switch/hook untuk auto-klasifikasi tipe memori.
// Di-set oleh package/file terpisah (editable) agar core beku tidak perlu diubah lagi.
var MemTypeClassifierHook func(content, wing, room, currentType string) string

type BrainDrawer struct {
	ID          string  `json:"id"`
	Content     string  `json:"content"`
	Wing        string  `json:"wing"`
	Room        string  `json:"room"`
	MemType     string  `json:"mem_type"`
	Importance  float64 `json:"importance"`
	Amplitude   int     `json:"amplitude"`
	ContentHash string  `json:"content_hash"`
	Source      string  `json:"source"`
	Quarantined bool    `json:"quarantined"`
	Confidence  float64 `json:"confidence"`
	CreatedAt   string  `json:"created_at"`
}

type BrainHit struct {
	DrawerID string  `json:"drawer_id"`
	Wing     string  `json:"wing"`
	Room     string  `json:"room"`
	MemType  string  `json:"mem_type"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
}

const (
	maxBrainContentBytes = 16 * 1024
	defaultLocalBrainK   = 5
	maxLocalBrainK       = 10
	maxBrainSnippetChars = 1000
)

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
	if MemTypeClassifierHook != nil {
		memType = MemTypeClassifierHook(content, wing, room, memType)
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

	if _, err = s.db.Exec(
		`INSERT INTO brain_fts (drawer_id, content, wing, room) VALUES (?,?,?,?)`,
		id, content, wing, room,
	); err != nil {
		return "", false, fmt.Errorf("insert fts: %w", err)
	}
	return id, true, nil
}

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
		return BrainDrawer{}, false, nil
	}
	d.Quarantined = quar == 1
	return d, true, nil
}

func (s *Store) CountBrainDrawers() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureBrainSchema()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM brain_drawers WHERE deleted_at IS NULL`).Scan(&n)
	return n, err
}

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

func joinFTS(tokens []string, op string) string {
	return strings.Join(tokens, " "+op+" ")
}
