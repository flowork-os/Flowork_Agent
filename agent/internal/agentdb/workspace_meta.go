// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

var errSkipAll = errors.New("workspace_meta: stop walk (cap reached)")

var CategoryWhitelist = map[string]struct{}{
	"tools":    {},
	"job":      {},
	"document": {},
	"media":    {},
	"cache":    {},
	"log":      {},
}

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

func (s *Store) RegisterMeta(category, path, description, contentHash string, sizeBytes int64, shareable bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if category == "" || path == "" {
		return 0, fmt.Errorf("category + path required")
	}
	if _, ok := CategoryWhitelist[category]; !ok {
		return 0, fmt.Errorf("category %q not in whitelist", category)
	}

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

type RebuildIndexReport struct {
	StartedAt   string   `json:"started_at"`
	FinishedAt  string   `json:"finished_at"`
	Scanned     int      `json:"scanned"`
	Registered  int      `json:"registered"`
	Updated     int      `json:"updated"`
	SoftDeleted int      `json:"soft_deleted"`
	SkippedCat  int      `json:"skipped_category"`
	Errors      []string `json:"errors,omitempty"`
}

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
		maxFiles     = 5000
		maxHashBytes = 100 * 1024 * 1024
	)

	scanned := map[string]struct{}{}

	for category := range CategoryWhitelist {
		catRoot := filepath.Join(workspaceRoot, category)
		if _, err := os.Stat(catRoot); err != nil {
			rep.SkippedCat++
			continue
		}
		werr := filepath.Walk(catRoot, func(absPath string, info os.FileInfo, werr error) error {
			if werr != nil {
				rep.Errors = append(rep.Errors, "walk "+absPath+": "+werr.Error())
				return nil
			}

			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if rep.Scanned >= maxFiles {
				return errSkipAll
			}
			rep.Scanned++

			rel, rerr := filepath.Rel(workspaceRoot, absPath)
			if rerr != nil {
				rep.Errors = append(rep.Errors, "rel "+absPath+": "+rerr.Error())
				return nil
			}

			if strings.Contains(rel, "..") {
				rep.Errors = append(rep.Errors, "rejected escaped path: "+rel)
				return nil
			}

			scanned[filepath.ToSlash(rel)] = struct{}{}

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

			catRel, rerr2 := filepath.Rel(catRoot, absPath)
			if rerr2 != nil {
				catRel = filepath.Base(absPath)
			}
			catRel = filepath.ToSlash(catRel)

			if strings.Contains(catRel, "..") {
				rep.Errors = append(rep.Errors, "rejected escaped catRel: "+catRel)
				return nil
			}

			existing, _ := s.lookupMetaNoLock(category, catRel)
			if existing.ID != 0 && existing.ContentHash == hash && existing.SizeBytes == info.Size() {

				return nil
			}

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
			break
		}
	}

	deleted, derr := s.softDeleteMissingMetaNoLock(scanned)
	if derr != nil {
		rep.Errors = append(rep.Errors, "soft-delete missing: "+derr.Error())
	}
	rep.SoftDeleted = int(deleted)

	rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return rep
}

func (s *Store) lookupMetaNoLock(category, path string) (WorkspaceMeta, error) {

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
