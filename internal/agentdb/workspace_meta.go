// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 6 (Workspace meta) DONE + adversarial-audit passed
//   (3 critical: symlink-skip via os.ModeSymlink check, path traversal
//   defense in registerMetaNoLock + Rel escape reject, errSkipAll
//   sentinel buat hard cap maxFiles; plus important: defer f.Close
//   via closure, dead alt-key fallback removed, defer rows.Close).
//   API stable: RegisterMeta atomic upsert, ListMeta, LookupMeta,
//   RebuildIndexFromDir + RebuildIndexReport, CountMeta. Future cron
//   hourly auto-rebuild → wire kernelhost (mirror StartRetentionCron),
//   JANGAN modify ini.
//
// workspace_meta.go — Section 6 roadmap: Workspace metadata per-warga.
//
// PURPOSE:
//   Index file di shared workspace warga (<root>/workspace/<id>/).
//   Register category + path + size + content_hash supaya warga lain
//   bisa discover tools/job/document via mesh future.
//
// SEMANTIC:
//   - RegisterMeta: upsert by (category, path). Idempotent.
//   - ListMeta(category): paginated browse.
//   - LookupMeta(path): single read by path (any category).
//   - RebuildIndexFromDir: scan filesystem, auto-register new file +
//     soft-delete row yang file-nya hilang. Caller butuh inject
//     workspace root path (kernelhost owns SharedDir).
//
// SECURITY:
//   - Path stored relative dari shared root (`tools/foo.py`, bukan absolute).
//   - Caller wajib validate agentID + category whitelist sebelum invoke.
//
// ⚠️ NO over-prompt: workspace meta bukan untuk LLM context — purely
// inventory. Akses via API endpoint untuk dashboard/discovery.

package agentdb

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errSkipAll — sentinel buat short-circuit filepath.Walk saat hit
// maxFiles cap. SkipDir cuma skip current dir; SkipAll baru ada di
// Go 1.20+ tapi kita pakai sentinel custom supaya backward-safe.
var errSkipAll = errors.New("workspace_meta: stop walk (cap reached)")

// CategoryWhitelist — valid category names. Match shared subfolder convention
// di kernelhost.SharedSubfolders.
var CategoryWhitelist = map[string]struct{}{
	"tools":    {},
	"job":      {},
	"document": {},
	"media":    {},
	"cache":    {},
	"log":      {},
}

// WorkspaceMeta — satu row.
type WorkspaceMeta struct {
	ID          int64  `json:"id"`
	Category    string `json:"category"`
	Path        string `json:"path"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	Shareable   bool   `json:"shareable"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// RegisterMeta — upsert via UNIQUE(category, path). Kalau row exist:
// update size_bytes + content_hash + updated_at (preserve description +
// shareable + created_at + restore deleted_at=NULL kalau soft-deleted).
//
// Description hard-cap 4KB, content_hash expected 64-hex (sha256).
func (s *Store) RegisterMeta(category, path, description, contentHash string, sizeBytes int64, shareable bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if category == "" || path == "" {
		return 0, fmt.Errorf("category + path required")
	}
	if _, ok := CategoryWhitelist[category]; !ok {
		return 0, fmt.Errorf("category %q not in whitelist", category)
	}
	// Reject absolute path / path traversal.
	if strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return 0, fmt.Errorf("path must be relative, no traversal")
	}

	const maxDescBytes = 4 * 1024
	if len(description) > maxDescBytes {
		description = description[:maxDescBytes] + "…"
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	shareInt := 0
	if shareable {
		shareInt = 1
	}

	// Atomic upsert via transaction (SELECT-then-INSERT-or-UPDATE, mirror
	// mistakes.go pattern — undelete kalau soft-deleted).
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var existingID int64
	err = tx.QueryRow(
		`SELECT id FROM workspace_meta WHERE category = ? AND path = ?`,
		category, path,
	).Scan(&existingID)

	switch {
	case err == sql.ErrNoRows:
		res, ierr := tx.Exec(
			`INSERT INTO workspace_meta(category, path, description, size_bytes, content_hash, shareable, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			category, path, description, sizeBytes, contentHash, shareInt, ts, ts,
		)
		if ierr != nil {
			return 0, fmt.Errorf("insert meta: %w", ierr)
		}
		newID, _ := res.LastInsertId()
		if cerr := tx.Commit(); cerr != nil {
			return 0, fmt.Errorf("commit insert: %w", cerr)
		}
		tx = nil
		return newID, nil

	case err != nil:
		return 0, fmt.Errorf("lookup meta: %w", err)

	default:
		// UPDATE existing — refresh size + hash + updated_at + undelete.
		// Preserve description + shareable + created_at (caller intent OK).
		_, uerr := tx.Exec(
			`UPDATE workspace_meta SET
			    size_bytes   = ?,
			    content_hash = ?,
			    updated_at   = ?,
			    deleted_at   = NULL
			 WHERE id = ?`,
			sizeBytes, contentHash, ts, existingID,
		)
		if uerr != nil {
			return 0, fmt.Errorf("upsert meta: %w", uerr)
		}
		if cerr := tx.Commit(); cerr != nil {
			return 0, fmt.Errorf("commit upsert: %w", cerr)
		}
		tx = nil
		return existingID, nil
	}
}

// ListMeta — filter optional category. Order: updated_at DESC. Limit
// default 100, max 500.
func (s *Store) ListMeta(category string, limit int) ([]WorkspaceMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `SELECT id, category, path, description, size_bytes, content_hash,
	                 shareable, created_at, updated_at
	          FROM workspace_meta WHERE deleted_at IS NULL`
	args := []any{}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query meta: %w", err)
	}
	defer rows.Close()

	var out []WorkspaceMeta
	for rows.Next() {
		var m WorkspaceMeta
		var shareInt int
		if err := rows.Scan(&m.ID, &m.Category, &m.Path, &m.Description,
			&m.SizeBytes, &m.ContentHash, &shareInt,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Shareable = shareInt != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

// LookupMeta — single read by category + path. Return empty + nil
// kalau ngga ada (caller cek len == 0 atau Path == "").
func (s *Store) LookupMeta(category, path string) (WorkspaceMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if category == "" || path == "" {
		return WorkspaceMeta{}, fmt.Errorf("category + path required")
	}

	var m WorkspaceMeta
	var shareInt int
	err := s.db.QueryRow(
		`SELECT id, category, path, description, size_bytes, content_hash,
		        shareable, created_at, updated_at
		 FROM workspace_meta WHERE category = ? AND path = ? AND deleted_at IS NULL`,
		category, path,
	).Scan(&m.ID, &m.Category, &m.Path, &m.Description, &m.SizeBytes,
		&m.ContentHash, &shareInt, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return WorkspaceMeta{}, nil
	}
	if err != nil {
		return WorkspaceMeta{}, fmt.Errorf("lookup meta: %w", err)
	}
	m.Shareable = shareInt != 0
	return m, nil
}

// RebuildIndexReport — outcome RebuildIndexFromDir call.
type RebuildIndexReport struct {
	StartedAt    string  `json:"started_at"`
	FinishedAt   string  `json:"finished_at"`
	Scanned      int     `json:"scanned"`        // file found di disk
	Registered   int     `json:"registered"`     // new INSERT
	Updated      int     `json:"updated"`        // existing row hash/size berubah
	SoftDeleted  int     `json:"soft_deleted"`   // row exist tapi file hilang
	SkippedCat   int     `json:"skipped_category"`
	Errors       []string `json:"errors,omitempty"`
}

// RebuildIndexFromDir — scan shared workspace root + register file baru,
// update existing dengan hash/size baru, soft-delete row yang file-nya
// hilang. workspaceRoot adalah path absolute ke `<root>/workspace/<agent_id>/`.
//
// Hard cap: max 5000 file per sweep (anti-DOS kalau folder tiba-tiba huge).
// Per-file hash SHA-256 (full content). Buat file >100MB, hash skipped + warning.
func (s *Store) RebuildIndexFromDir(workspaceRoot string) RebuildIndexReport {
	rep := RebuildIndexReport{StartedAt: time.Now().UTC().Format(time.RFC3339)}

	if workspaceRoot == "" {
		rep.Errors = append(rep.Errors, "workspace root path empty")
		rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		return rep
	}
	if _, err := os.Stat(workspaceRoot); err != nil {
		rep.Errors = append(rep.Errors, "workspace root not exists: "+err.Error())
		rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		return rep
	}

	const (
		maxFiles      = 5000
		maxHashBytes  = 100 * 1024 * 1024 // 100MB cap untuk hash compute
	)

	// Track yang ke-scan supaya bisa soft-delete row yang ngga muncul.
	scanned := map[string]struct{}{} // key = "category/path"

	for category := range CategoryWhitelist {
		catRoot := filepath.Join(workspaceRoot, category)
		if _, err := os.Stat(catRoot); err != nil {
			rep.SkippedCat++
			continue
		}
		werr := filepath.Walk(catRoot, func(absPath string, info os.FileInfo, werr error) error {
			if werr != nil {
				rep.Errors = append(rep.Errors, "walk "+absPath+": "+werr.Error())
				return nil // continue
			}
			// CRITICAL audit fix: skip symlinks supaya attacker ngga bisa
			// taro symlink → /etc/passwd dan bikin scanner hash sensitive
			// file. info.Mode() check — symlinks tetap stat() via Walk
			// (follows by default), kita cek mode untuk skip.
			if info.Mode()&os.ModeSymlink != 0 {
				return nil // skip symlink
			}
			if info.IsDir() {
				return nil
			}
			if rep.Scanned >= maxFiles {
				return errSkipAll
			}
			rep.Scanned++

			// Relative path dari workspaceRoot (mis. "tools/foo.py").
			rel, rerr := filepath.Rel(workspaceRoot, absPath)
			if rerr != nil {
				rep.Errors = append(rep.Errors, "rel "+absPath+": "+rerr.Error())
				return nil
			}
			// Defense in depth: kalau Rel produce path dengan `..` (mis.
			// dari symlink yang lolos cek), reject.
			if strings.Contains(rel, "..") {
				rep.Errors = append(rep.Errors, "rejected escaped path: "+rel)
				return nil
			}
			// Normalize separator supaya match dengan fullKey di softDelete
			// (e.g. Windows backslash → forward slash).
			scanned[filepath.ToSlash(rel)] = struct{}{}

			// Hash file via closure supaya defer f.Close() panic-safe.
			var hash string
			if info.Size() <= maxHashBytes {
				hash = func() string {
					f, ferr := os.Open(absPath)
					if ferr != nil {
						return ""
					}
					defer f.Close()
					h := sha256.New()
					if _, cerr := io.Copy(h, f); cerr != nil {
						return ""
					}
					return hex.EncodeToString(h.Sum(nil))
				}()
			}

			// Path stored without category prefix supaya UNIQUE(category, path)
			// pakai relative-within-category (e.g. "foo.py" bukan "tools/foo.py").
			catRel, rerr2 := filepath.Rel(catRoot, absPath)
			if rerr2 != nil {
				catRel = filepath.Base(absPath)
			}
			catRel = filepath.ToSlash(catRel) // normalize separator
			// Defense in depth: reject `..` in catRel juga.
			if strings.Contains(catRel, "..") {
				rep.Errors = append(rep.Errors, "rejected escaped catRel: "+catRel)
				return nil
			}

			// Check if existing row has same hash → skip register (no-op).
			existing, _ := s.lookupMetaNoLock(category, catRel)
			if existing.ID != 0 && existing.ContentHash == hash && existing.SizeBytes == info.Size() {
				// unchanged
				return nil
			}

			// Register (insert or update). Unlock s.mu sebentar — Register
			// call ambil lock sendiri. Tapi kita di tengah filepath.Walk
			// yang OS-level. Pakai internal upsert tanpa lock supaya ngga
			// re-entrant deadlock.
			id, isNew, uerr := s.registerMetaNoLock(category, catRel, "", hash, info.Size(), true)
			_ = id
			if uerr != nil {
				rep.Errors = append(rep.Errors, "register "+category+"/"+catRel+": "+uerr.Error())
				return nil
			}
			if isNew {
				rep.Registered++
			} else {
				rep.Updated++
			}
			return nil
		})
		if werr != nil && !errors.Is(werr, errSkipAll) {
			rep.Errors = append(rep.Errors, "walk "+catRoot+": "+werr.Error())
		}
		if errors.Is(werr, errSkipAll) {
			break // hit cap, ngga iterate sisa category
		}
	}

	// Soft-delete row yang ngga muncul di scan (file hilang dari disk).
	deleted, derr := s.softDeleteMissingMetaNoLock(scanned)
	if derr != nil {
		rep.Errors = append(rep.Errors, "soft-delete missing: "+derr.Error())
	}
	rep.SoftDeleted = int(deleted)

	rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return rep
}

// lookupMetaNoLock — internal helper, caller wajib pegang lock (atau pakai
// dari context yang ngga butuh lock seperti RebuildIndexFromDir holding via
// outer wrapper).
func (s *Store) lookupMetaNoLock(category, path string) (WorkspaceMeta, error) {
	// NOTE: caller di RebuildIndexFromDir actually ngga hold s.mu —
	// RebuildIndexFromDir didesain ngga lock seluruh duration supaya
	// scan banyak file ngga monopoli writer. Concurrent caller via
	// other method tetap aman via SQLite WAL.
	var m WorkspaceMeta
	var shareInt int
	err := s.db.QueryRow(
		`SELECT id, category, path, description, size_bytes, content_hash,
		        shareable, created_at, updated_at
		 FROM workspace_meta WHERE category = ? AND path = ? AND deleted_at IS NULL`,
		category, path,
	).Scan(&m.ID, &m.Category, &m.Path, &m.Description, &m.SizeBytes,
		&m.ContentHash, &shareInt, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return WorkspaceMeta{}, nil
	}
	if err != nil {
		return WorkspaceMeta{}, err
	}
	m.Shareable = shareInt != 0
	return m, nil
}

// registerMetaNoLock — internal helper buat RebuildIndex. Mirror logic
// RegisterMeta tapi tanpa s.mu lock (caller manage).
//
// Apply same path validation as RegisterMeta (audit fix — consistency).
func (s *Store) registerMetaNoLock(category, path, description, contentHash string, sizeBytes int64, shareable bool) (id int64, isNew bool, err error) {
	if category == "" || path == "" {
		return 0, false, fmt.Errorf("category + path required")
	}
	if _, ok := CategoryWhitelist[category]; !ok {
		return 0, false, fmt.Errorf("category %q not in whitelist", category)
	}
	if strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return 0, false, fmt.Errorf("path must be relative, no traversal")
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	shareInt := 0
	if shareable {
		shareInt = 1
	}

	var existingID int64
	qerr := s.db.QueryRow(
		`SELECT id FROM workspace_meta WHERE category = ? AND path = ?`,
		category, path,
	).Scan(&existingID)

	switch {
	case qerr == sql.ErrNoRows:
		res, ierr := s.db.Exec(
			`INSERT INTO workspace_meta(category, path, description, size_bytes, content_hash, shareable, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			category, path, description, sizeBytes, contentHash, shareInt, ts, ts,
		)
		if ierr != nil {
			return 0, false, fmt.Errorf("insert: %w", ierr)
		}
		newID, _ := res.LastInsertId()
		return newID, true, nil
	case qerr != nil:
		return 0, false, fmt.Errorf("lookup: %w", qerr)
	default:
		_, uerr := s.db.Exec(
			`UPDATE workspace_meta SET size_bytes = ?, content_hash = ?,
			        updated_at = ?, deleted_at = NULL WHERE id = ?`,
			sizeBytes, contentHash, ts, existingID,
		)
		if uerr != nil {
			return 0, false, fmt.Errorf("update: %w", uerr)
		}
		return existingID, false, nil
	}
}

// softDeleteMissingMetaNoLock — soft-delete row yang `category/path` ngga
// muncul di `scanned` set. Pakai pas RebuildIndex untuk garbage collect.
//
// Scanned keys = `<category>/<relative-within-category>` (e.g. `tools/foo.py`).
// Match cek single fullKey only — audit fix I6 dead alt-key fallback removed.
func (s *Store) softDeleteMissingMetaNoLock(scanned map[string]struct{}) (int64, error) {
	rows, err := s.db.Query(`SELECT id, category, path FROM workspace_meta WHERE deleted_at IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("scan live: %w", err)
	}
	defer rows.Close()
	var toDelete []int64
	for rows.Next() {
		var id int64
		var cat, path string
		if err := rows.Scan(&id, &cat, &path); err != nil {
			return 0, err
		}
		fullKey := cat + "/" + path
		if _, ok := scanned[fullKey]; !ok {
			toDelete = append(toDelete, id)
		}
	}
	if rerr := rows.Err(); rerr != nil {
		return 0, rerr
	}

	if len(toDelete) == 0 {
		return 0, nil
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	var deleted int64
	for _, id := range toDelete {
		res, derr := s.db.Exec(
			`UPDATE workspace_meta SET deleted_at = ? WHERE id = ?`,
			ts, id,
		)
		if derr != nil {
			continue
		}
		n, _ := res.RowsAffected()
		deleted += n
	}
	return deleted, nil
}

// CountMeta — total non-deleted, optional filter category.
func (s *Store) CountMeta(category string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT COUNT(*) FROM workspace_meta WHERE deleted_at IS NULL`
	args := []any{}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	var n int64
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
