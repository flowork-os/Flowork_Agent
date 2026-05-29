// Package db — softdelete.go: visi append-only doctrine.
//
// Sprint 3.5d (Phase 1, 2026-05-02 by antigravity):
// AI butuh inget kesalahan supaya ngga ulang. Hard DELETE = AI lupa = melanggar
// visi self-learning. Sebagai gantinya, soft-delete: tandai row dengan
// `deleted_at` + `deleted_by` timestamp, ngga benar-benar hapus.
//
// Caller (GUI handler) ganti `db.Exec("DELETE FROM ...")` →
//   `db.SoftDelete(table, id, "ayah")` — preserves FQP-12 append-only.
//
// AI baca data: default WHERE deleted_at IS NULL (auto filter active rows).
// Audit/restore: WHERE deleted_at IS NOT NULL (lihat history).
//
// Tabel yang adopt pattern: memories, skills, recordings, tool_patterns,
// prompt_templates, tasks, drawers (perbarui schema dulu via migration).

package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// SoftDelete tandai row sebagai deleted tanpa benar-benar hapus.
// FQP-12 append-only preserved. Caller boleh assume table punya kolom:
//   - deleted_at TIMESTAMP NULL
//   - deleted_by TEXT NULL
//
// Migration helper EnsureSoftDeleteColumns auto-add columns kalau belum ada.
//
// Args:
//   - tx: optional transaction (nil = use default db connection)
//   - table: nama tabel (whitelist enforcement, anti-injection)
//   - idCol: kolom primary key (e.g. "id", "name")
//   - idVal: value primary key
//   - deletedBy: identifier (e.g. "ayah", "merpati", "system")
//
// Returns affected rows + err.
func SoftDelete(db *sql.DB, table, idCol string, idVal any, deletedBy string) (int64, error) {
	if !isAllowedSoftDeleteTable(table) {
		return 0, fmt.Errorf("softdelete: table %q not whitelisted (anti-injection)", table)
	}
	if !isAllowedIDCol(idCol) {
		return 0, fmt.Errorf("softdelete: idCol %q not whitelisted", idCol)
	}
	q := fmt.Sprintf(`UPDATE %s SET deleted_at = ?, deleted_by = ? WHERE %s = ? AND deleted_at IS NULL`, table, idCol)
	res, err := db.Exec(q, time.Now().UTC().Format(time.RFC3339), deletedBy, idVal)
	if err != nil {
		return 0, fmt.Errorf("softdelete %s: %w", table, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("softdelete rows affected: %w", err)
	}
	return n, nil
}

// SoftRestore unmark row deleted (reverse SoftDelete).
func SoftRestore(db *sql.DB, table, idCol string, idVal any) (int64, error) {
	if !isAllowedSoftDeleteTable(table) {
		return 0, fmt.Errorf("softrestore: table %q not whitelisted", table)
	}
	if !isAllowedIDCol(idCol) {
		return 0, fmt.Errorf("softrestore: idCol %q not whitelisted", idCol)
	}
	q := fmt.Sprintf(`UPDATE %s SET deleted_at = NULL, deleted_by = NULL WHERE %s = ?`, table, idCol)
	res, err := db.Exec(q, idVal)
	if err != nil {
		return 0, fmt.Errorf("softrestore %s: %w", table, err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// EnsureSoftDeleteColumns auto-add kolom deleted_at + deleted_by ke tabel
// kalau belum ada. Idempotent — safe to call setiap boot.
func EnsureSoftDeleteColumns(db *sql.DB, table string) error {
	if !isAllowedSoftDeleteTable(table) {
		return fmt.Errorf("ensure softdelete cols: table %q not whitelisted", table)
	}
	// SQLite: try ALTER TABLE ADD COLUMN, ignore "duplicate column" error.
	for _, col := range []struct{ name, ddl string }{
		{"deleted_at", "ALTER TABLE %s ADD COLUMN deleted_at TIMESTAMP"},
		{"deleted_by", "ALTER TABLE %s ADD COLUMN deleted_by TEXT"},
	} {
		q := fmt.Sprintf(col.ddl, table)
		if _, err := db.Exec(q); err != nil {
			// SQLite returns "duplicate column" — acceptable.
			if !isDuplicateColumnErr(err) {
				return fmt.Errorf("ensure %s.%s: %w", table, col.name, err)
			}
		}
	}
	// Index for efficient WHERE deleted_at IS NULL queries.
	idxName := fmt.Sprintf("idx_%s_deleted_at", table)
	_, _ = db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s(deleted_at)`, idxName, table))
	return nil
}

// allowedSoftDeleteTables — whitelist anti SQL injection. Caller ngga bisa
// pass arbitrary table name.
//
// FIX #58 effekdomino.md: previously hardcoded const map. Per [AI.2] precedent
// constitution gap (caller pakai SoftDelete sebelum tabel ke-whitelist =
// silent fail), tambah AllowSoftDelete(table, idCol) helper register baru
// dynamic boot-time tanpa recompile. Plus-pattern Plug-and-play DB-driven
// (P-1.3 doktrin) — masa depan migrate ke settings DB row.
//
// Default seed (hardcoded) tetap untuk backward compat + bootstrap.
var (
	softDeleteMu          sync.RWMutex
	allowedSoftDeleteTables = map[string]bool{
		"memories":         true, // Brain memories (BUG-C2/C3 fix)
		"skills":           true, // Brain skills (W31 fix)
		"recordings":       true, // Chat recordings (BUG-C18 fix)
		"tool_patterns":    true, // Tool usage patterns (W32 fix)
		"prompt_templates": true, // Prompt library (W33 fix)
		"tasks":            true, // Calendar tasks (W34 fix)
		"drawers":          true, // Brain drawers (W37 fix)
		"agents":           true, // Agent retire soft-delete (BUG-C4 fix)
		"constitution":     true, // Constitution sacrosanct (BUG-C6 Phase 1)
	}

	allowedIDCols = map[string]bool{
		"id":       true,
		"name":     true,
		"key":      true,
		"uid":      true,
		"agent_id": true, // cascade retire: soft-delete WHERE agent_id = ?
	}
)

func isAllowedSoftDeleteTable(t string) bool {
	softDeleteMu.RLock()
	defer softDeleteMu.RUnlock()
	return allowedSoftDeleteTables[t]
}

func isAllowedIDCol(c string) bool {
	softDeleteMu.RLock()
	defer softDeleteMu.RUnlock()
	return allowedIDCols[c]
}

// AllowSoftDelete register tabel baru ke whitelist soft-delete dynamic.
// Idempotent — call beberapa kali OK. Pakai di boot-time sebelum SeedX
// supaya tabel baru langsung pickup tanpa recompile.
//
// Caller pattern (per finding #58):
//
//	func init() { braindb.AllowSoftDelete("notes", "id") }
//
// Atau pas tabel diregistrasi di schema.go saat module load.
func AllowSoftDelete(table, idCol string) {
	softDeleteMu.Lock()
	defer softDeleteMu.Unlock()
	allowedSoftDeleteTables[table] = true
	if idCol != "" {
		allowedIDCols[idCol] = true
	}
}

// isDuplicateColumnErr deteksi error "duplicate column name" dari SQLite.
func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, pattern := range []string{"duplicate column", "already exists", "duplicate column name"} {
		if containsLower(msg, pattern) {
			return true
		}
	}
	return false
}

func containsLower(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
