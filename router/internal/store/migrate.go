// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
)

type Migration struct {
	ID   int
	Name string
	SQL  string
}

var (
	migrationsMu sync.Mutex
	migrations   []Migration
)

func RegisterMigration(m Migration) {
	migrationsMu.Lock()
	defer migrationsMu.Unlock()
	migrations = append(migrations, m)
}

func applyMigrations(d *sql.DB) error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schemaMigrations (
		id        INTEGER PRIMARY KEY,
		name      TEXT NOT NULL,
		appliedAt TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create schemaMigrations: %w", err)
	}

	migrationsMu.Lock()
	pending := append([]Migration(nil), migrations...)
	migrationsMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })

	applied := map[int]bool{}
	rows, err := d.Query(`SELECT id FROM schemaMigrations`)
	if err != nil {
		return fmt.Errorf("query schemaMigrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			applied[id] = true
		}
	}
	rows.Close()

	var todo []Migration
	for _, m := range pending {
		if !applied[m.ID] {
			todo = append(todo, m)
		}
	}
	if len(todo) == 0 {
		return nil
	}

	_, _ = backupWithConn(d, "pre-migrate", defaultKeepBackups)

	for _, m := range todo {
		tx, err := d.Begin()
		if err != nil {
			return fmt.Errorf("begin %d: %w", m.ID, err)
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d %q: %w", m.ID, m.Name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schemaMigrations (id, name) VALUES (?, ?)`, m.ID, m.Name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.ID, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %d: %w", m.ID, err)
		}
	}
	return nil
}

type MigrationStatus struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Applied   bool   `json:"applied"`
	AppliedAt string `json:"appliedAt,omitempty"`
}

func ListMigrationStatus(d *sql.DB) ([]MigrationStatus, error) {
	migrationsMu.Lock()
	all := append([]Migration(nil), migrations...)
	migrationsMu.Unlock()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	applied := map[int]string{}
	rows, err := d.Query(`SELECT id, appliedAt FROM schemaMigrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var at string
		if err := rows.Scan(&id, &at); err == nil {
			applied[id] = at
		}
	}
	rows.Close()

	out := make([]MigrationStatus, 0, len(all))
	for _, m := range all {
		s := MigrationStatus{ID: m.ID, Name: m.Name}
		if at, ok := applied[m.ID]; ok {
			s.Applied = true
			s.AppliedAt = at
		}
		out = append(out, s)
	}
	return out, nil
}
