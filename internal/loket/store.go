// === FROZEN (kernel inti) — DO NOT MODIFY. Kernel FREEZE v1 (2026-06-07). ===
// Owner: Aola Sahidin (Mr.Dev). Bagian microkernel "papan kosong" abadi; checksum
// dipin di KERNEL_FREEZE.md. Ubah = unfreeze eksplisit owner + update manifest.

package loket

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a module's private, isolated storage: one SQLite file inside the
// module's own folder. It backs three families of capabilities:
//
//	store.kv.*    — small config keys
//	store.doc.*   — structured records grouped into collections
//	store.brain.* — a local semantic memory (FTS5 + dedup) — the "ant's" own brain
//
// The schema is deliberately clean and minimal: the eternal kernel's storage
// must not carry feature-specific tables. The SQLite setup (WAL + busy_timeout)
// and the brain dedup-by-hash pattern are adapted from the proven agentdb layer.
type Store struct {
	mu sync.Mutex
	db *sql.DB
}

// OpenStore opens (creating if needed) the SQLite file at path and ensures the
// schema. The caller owns the returned Store and must Close it.
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir store dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping store: %w", err)
	}
	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kv (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS doc (
			collection TEXT NOT NULL,
			id         TEXT NOT NULL,
			body       TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (collection, id)
		)`,
		`CREATE TABLE IF NOT EXISTS brain_drawers (
			id           TEXT PRIMARY KEY,
			content      TEXT NOT NULL,
			wing         TEXT NOT NULL DEFAULT 'general',
			room         TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_brain_hash ON brain_drawers(content_hash)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS brain_fts USING fts5(
			drawer_id UNINDEXED, content, wing, room, tokenize='porter unicode61'
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("store schema: %w", err)
		}
	}
	return nil
}

// ── kv ───────────────────────────────────────────────────────────────────────

// KVGet returns the value for k and whether it was found.
func (s *Store) KVGet(k string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	err := s.db.QueryRow(`SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// KVSet upserts k=v.
func (s *Store) KVSet(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO kv(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

// KVDelete removes k.
func (s *Store) KVDelete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM kv WHERE k=?`, k)
	return err
}

// KVList returns keys with the given prefix (empty prefix = all), sorted.
func (s *Store) KVList(prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT k FROM kv WHERE k LIKE ? ORDER BY k`, escapeLike(prefix)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ── doc ──────────────────────────────────────────────────────────────────────

// DocPut upserts a record into a collection.
func (s *Store) DocPut(collection, id string, body json.RawMessage) error {
	if collection == "" || id == "" {
		return fmt.Errorf("doc: collection and id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO doc(collection,id,body,updated_at) VALUES(?,?,?,?)
		 ON CONFLICT(collection,id) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at`,
		collection, id, string(body), time.Now().UTC().Format(time.RFC3339))
	return err
}

// DocGet returns a record and whether it was found.
func (s *Store) DocGet(collection, id string) (json.RawMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body string
	err := s.db.QueryRow(`SELECT body FROM doc WHERE collection=? AND id=?`, collection, id).Scan(&body)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(body), true, nil
}

// DocDelete removes a record.
func (s *Store) DocDelete(collection, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM doc WHERE collection=? AND id=?`, collection, id)
	return err
}

// DocQuery returns up to limit records from a collection, newest first.
func (s *Store) DocQuery(collection string, limit int) ([]json.RawMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT body FROM doc WHERE collection=? ORDER BY updated_at DESC, id LIMIT ?`, collection, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(body))
	}
	return out, rows.Err()
}

// ── brain ────────────────────────────────────────────────────────────────────

const maxBrainContent = 16 * 1024

// BrainAdd stores a knowledge drawer. Dedup by content hash: identical content
// returns the existing id with added=false.
func (s *Store) BrainAdd(content, wing, room string) (id string, added bool, err error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", false, fmt.Errorf("brain: empty content")
	}
	if len(content) > maxBrainContent {
		content = content[:maxBrainContent]
	}
	if wing == "" {
		wing = "general"
	}
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	id = hash[:16]

	s.mu.Lock()
	defer s.mu.Unlock()
	var existing string
	if e := s.db.QueryRow(`SELECT id FROM brain_drawers WHERE content_hash=? LIMIT 1`, hash).Scan(&existing); e == nil && existing != "" {
		return existing, false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err = s.db.Exec(`INSERT OR IGNORE INTO brain_drawers(id,content,wing,room,content_hash,created_at) VALUES(?,?,?,?,?,?)`,
		id, content, wing, room, hash, now); err != nil {
		return "", false, err
	}
	if _, err = s.db.Exec(`INSERT INTO brain_fts(drawer_id,content,wing,room) VALUES(?,?,?,?)`, id, content, wing, room); err != nil {
		return "", false, err
	}
	return id, true, nil
}

// BrainHit is one search result.
type BrainHit struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Wing    string `json:"wing"`
	Room    string `json:"room"`
}

// BrainSearch runs an FTS5 query over the local brain, best matches first.
func (s *Store) BrainSearch(query string, k int) ([]BrainHit, error) {
	if k <= 0 || k > 20 {
		k = 5
	}
	match := sanitizeFTS(query)
	if match == "" {
		return []BrainHit{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT d.id, d.content, d.wing, d.room
		   FROM brain_fts f JOIN brain_drawers d ON d.id=f.drawer_id
		  WHERE brain_fts MATCH ? ORDER BY rank LIMIT ?`, match, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainHit{}
	for rows.Next() {
		var h BrainHit
		if err := rows.Scan(&h.ID, &h.Content, &h.Wing, &h.Room); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// sanitizeFTS turns raw user text into a safe FTS5 query: each word becomes a
// quoted literal OR-joined, so operator characters in input cannot cause a
// syntax error or injection.
func sanitizeFTS(q string) string {
	repl := strings.NewReplacer(`"`, " ", `*`, " ", `(`, " ", `)`, " ", `:`, " ", `^`, " ")
	fields := strings.Fields(repl.Replace(q))
	if len(fields) == 0 {
		return ""
	}
	for i, f := range fields {
		fields[i] = `"` + f + `"`
	}
	return strings.Join(fields, " OR ")
}

// escapeLike neutralises LIKE wildcards in a user-supplied prefix.
func escapeLike(s string) string {
	return strings.NewReplacer(`%`, "", `_`, "").Replace(s)
}
