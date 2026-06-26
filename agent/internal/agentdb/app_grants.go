// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) ensureAppGrantsSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS app_grants (
		  app_id     TEXT PRIMARY KEY,
		  granted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("ensure app_grants: %w", err)
	}
	return nil
}

func (s *Store) GrantApp(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAppGrantsSchema(); err != nil {
		return err
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return fmt.Errorf("app_id required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO app_grants (app_id, granted_at) VALUES (?, ?)
		 ON CONFLICT(app_id) DO NOTHING`, appID, now)
	return err
}

func (s *Store) RevokeApp(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAppGrantsSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM app_grants WHERE app_id = ?`, strings.TrimSpace(appID))
	return err
}

func (s *Store) ListAppGrants() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAppGrantsSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT app_id FROM app_grants ORDER BY app_id LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if serr := rows.Scan(&id); serr != nil {
			return nil, serr
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) AppGrantsSeeded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	_ = s.db.QueryRow(`SELECT v FROM meta WHERE k='app_grants_seeded'`).Scan(&v)
	return v == "1"
}

func (s *Store) MarkAppGrantsSeeded() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO meta(k, v) VALUES('app_grants_seeded', '1')
		 ON CONFLICT(k) DO UPDATE SET v='1'`)
	return err
}
