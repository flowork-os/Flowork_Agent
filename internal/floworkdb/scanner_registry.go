// scanner_registry.go — STATE install/uninstall scanner (owner-level, flowork.db).
//
// Katalog scanner (auditor defensif + nuclei pack ofensif) di-enumerate runtime;
// yang DISIMPEN cuma SET yang di-UNINSTALL (disabled). Default = installed (enabled)
// → tabel kosong = semua kepasang. Owner colok-cabut; angka "scanner aktif" ngikut.

package floworkdb

// ensureScannerRegistrySchema — tabel scanner_disabled (idempotent, lazy).
func (s *Store) ensureScannerRegistrySchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS scanner_disabled (
		id          TEXT PRIMARY KEY,
		disabled_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

// SetScannerInstalled — install (installed=true → hapus dari disabled) / uninstall
// (installed=false → masuk disabled). Idempotent.
func (s *Store) SetScannerInstalled(id string, installed bool) error {
	if err := s.ensureScannerRegistrySchema(); err != nil {
		return err
	}
	if installed {
		_, err := s.db.Exec(`DELETE FROM scanner_disabled WHERE id=?`, id)
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO scanner_disabled(id) VALUES(?)`, id)
	return err
}

// ListScannerDisabled — set id yang di-uninstall. Map buat lookup O(1) pas enumerate.
func (s *Store) ListScannerDisabled() (map[string]bool, error) {
	if err := s.ensureScannerRegistrySchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id FROM scanner_disabled`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out, rows.Err()
}
