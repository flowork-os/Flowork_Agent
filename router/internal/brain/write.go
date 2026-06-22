// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Brain drawer/embeddings/skills.

// Brain write side (option C: flow_router = sole owner).

package brain

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Write side of the brain — flow_router as the SOLE OWNER of the knowledge brain
// (option C). The read path (Open, mode=ro) stays the fast default; writes use a
// separate read-write connection opened lazily here. Admin edits go through the
// dashboard; organic learning goes through Ingest (compounding). Both respect the
// brain's append-only doctrine: soft-delete (tombstone), never hard DROP.

var (
	rwMu     sync.Mutex
	rwHandle *sql.DB
	rwPath   string
)

// OpenRW returns a shared read-write handle to the brain DB (lazy, cached).
// Distinct from Open() which is read-only. Used only for admin CRUD + ingest.
func OpenRW() (*sql.DB, error) {
	// Bootstrap the DB (mkdir brain/ + apply schema) BEFORE taking rwMu. A FRESH install (no brain DB
	// — the common case on a clean OS/USB/Android) otherwise fails every write with "unable to open
	// database file" / "no such table: drawers" (AddDrawer used to silently depend on some OTHER
	// endpoint calling EnsureSchema first). It MUST run before the lock: EnsureSchema → invalidateHandles
	// takes rwMu itself, so calling it WHILE holding rwMu self-deadlocks. Idempotent (CREATE … IF NOT EXISTS).
	if _, err := EnsureSchema(); err != nil {
		return nil, err
	}
	rwMu.Lock()
	defer rwMu.Unlock()
	p := DBPath()
	if rwHandle != nil && rwPath == p {
		return rwHandle, nil
	}
	if rwHandle != nil {
		_ = rwHandle.Close()
		rwHandle = nil
	}
	dsn := "file:" + p + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(8000)&_pragma=foreign_keys(0)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single writer
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	rwHandle = db
	rwPath = p
	return rwHandle, nil
}

// AddConstitution appends a new constitution rule (append-only). Returns new id.
func AddConstitution(ctx context.Context, section, content string, amplitude float64, source string) (int64, error) {
	if section == "" || content == "" {
		return 0, fmt.Errorf("section and content required")
	}
	if source == "" {
		source = "flow_router_admin"
	}
	db, err := OpenRW()
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `INSERT INTO constitution (source_file, section, content, amplitude)
		VALUES (?, ?, ?, ?)`, source, section, content, amplitude)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateConstitution edits an existing rule's content/amplitude (by id).
func UpdateConstitution(ctx context.Context, id int64, content string, amplitude float64) error {
	if content == "" {
		return fmt.Errorf("content required")
	}
	db, err := OpenRW()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE constitution SET content = ?, amplitude = ?
		WHERE id = ? AND deleted_at IS NULL`, content, amplitude, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("constitution id %d not found", id)
	}
	return nil
}

// SoftDeleteConstitution tombstones a rule (sets deleted_at) — never a hard DELETE,
// honoring the brain's append-only / FQP-12 doctrine.
func SoftDeleteConstitution(ctx context.Context, id int64) error {
	db, err := OpenRW()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE constitution SET deleted_at = ?, deleted_by = 'flow_router_admin'
		WHERE id = ? AND deleted_at IS NULL`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("constitution id %d not found or already deleted", id)
	}
	return nil
}

// AddDrawer files a new knowledge chunk into the Memory Palace (drawers + the
// FTS5 index, which has no sync trigger so both are written). Content-hash
// dedup: identical content is skipped. Returns (drawerID, added). This is the
// write primitive behind compounding ingest — the brain learning from use.
func AddDrawer(ctx context.Context, content, wing, room, memType string) (string, bool, error) {
	content = trimSpace(content)
	if content == "" {
		return "", false, fmt.Errorf("empty content")
	}
	if wing == "" {
		wing = "compounding"
	}
	if memType == "" {
		memType = "compounding"
	}
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	id := hash[:16]

	db, err := OpenRW()
	if err != nil {
		return "", false, err
	}
	// Dedup by content_hash (skip if a live drawer already has it).
	var exists string
	if err := db.QueryRowContext(ctx, `SELECT id FROM drawers WHERE content_hash = ? AND deleted_at IS NULL LIMIT 1`, hash).Scan(&exists); err == nil {
		return exists, false, nil // already known
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO drawers
		(id, content, wing, room, source_type, importance, content_hash, mem_type)
		VALUES (?, ?, ?, ?, 'compounding', 3.0, ?, ?)`, id, content, wing, room, hash, memType); err != nil {
		return "", false, fmt.Errorf("insert drawer: %w", err)
	}
	// Keep the FTS index in sync (no DB trigger does this).
	if _, err := db.ExecContext(ctx, `INSERT INTO memory_fts (drawer_id, content, wing, room, source_file)
		VALUES (?, ?, ?, ?, 'compounding')`, id, content, wing, room); err != nil {
		return "", false, fmt.Errorf("insert fts: %w", err)
	}
	return id, true, nil
}

// AddDrawerOpts — full param control untuk pipeline ingest (section 1
// roadmap). Pakai ini kalau caller butuh set source_type, source_file,
// importance, chunk_index, atau normalize_version explicit. Untuk caller
// simple-compounding (mis. /api/brain/ingest/run stub) pakai AddDrawer.
type AddDrawerOpts struct {
	Content          string  // wajib
	Wing             string  // default "compounding"
	Room             string  // free-form room/section
	SourceType       string  // 'manual' | 'chat' | 'doc' | 'federation' | 'compounding'
	SourceFile       string  // path/identifier asal (kosong = inline)
	MemType          string  // default "compounding" (project|antibody|fact|skill...)
	Importance       float64 // 0-10 scale; <= 0 → keep DB default (3.0)
	ChunkIndex       int     // 0 = atomic; > 0 = chunk N dari doc panjang
	NormalizeVersion int     // 0 → keep DB default (1)
}

// AddDrawerFull adalah versi extended dari AddDrawer dengan kontrol penuh
// atas semua kolom drawer. Dedupe content_hash + sync ke memory_fts sama
// kaya AddDrawer — caller tetap dapet drawer ID + flag `added` (false kalau
// content sudah ada di brain).
//
// Source: roadmap.md Section 1 (Ingestion pipeline).
func AddDrawerFull(ctx context.Context, opts AddDrawerOpts) (string, bool, error) {
	content := trimSpace(opts.Content)
	if content == "" {
		return "", false, fmt.Errorf("empty content")
	}
	wing := opts.Wing
	if wing == "" {
		wing = "compounding"
	}
	sourceType := opts.SourceType
	if sourceType == "" {
		sourceType = "manual"
	}
	memType := opts.MemType
	if memType == "" {
		memType = "compounding"
	}
	importance := opts.Importance
	if importance <= 0 {
		importance = 3.0
	}
	normalizeVer := opts.NormalizeVersion
	if normalizeVer <= 0 {
		normalizeVer = 1
	}

	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	id := hash[:16]

	db, err := OpenRW()
	if err != nil {
		return "", false, err
	}
	// Dedup by content_hash. Kalau live drawer udah punya hash sama → return
	// ID existing-nya (idempotent).
	var exists string
	if err := db.QueryRowContext(ctx, `SELECT id FROM drawers WHERE content_hash = ? AND deleted_at IS NULL LIMIT 1`, hash).Scan(&exists); err == nil {
		return exists, false, nil
	}

	// Drawer + FTS insert atomic dalam satu transaction. Tanpa ini, kalau FTS
	// insert gagal setelah drawer commit, drawer ada di DB tapi ngga
	// searchable → silent inconsistency.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO drawers
		(id, content, wing, room, source_file, source_type, chunk_index, importance, normalize_version, content_hash, mem_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, content, wing, opts.Room, opts.SourceFile, sourceType, opts.ChunkIndex,
		importance, normalizeVer, hash, memType,
	); err != nil {
		return "", false, fmt.Errorf("insert drawer: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_fts (drawer_id, content, wing, room, source_file)
		VALUES (?, ?, ?, ?, ?)`, id, content, wing, opts.Room, opts.SourceFile); err != nil {
		return "", false, fmt.Errorf("insert fts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit tx: %w", err)
	}
	tx = nil // skip rollback di defer setelah commit sukses
	return id, true, nil
}

// trimSpace — local helper (avoids importing strings just for this).
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
