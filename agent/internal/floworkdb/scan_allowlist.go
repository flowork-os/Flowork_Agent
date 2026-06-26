// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

package floworkdb

import (
	"database/sql"
	"fmt"
	"strings"
)

type AllowEntry struct {
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Note    string `json:"note"`
	AddedAt string `json:"added_at"`
}

func validAllowKind(k string) bool { return k == "exec" || k == "target" }

func (s *Store) EnsureScanSchema() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS scan_allowlist (
		kind     TEXT NOT NULL,
		value    TEXT NOT NULL,
		note     TEXT NOT NULL DEFAULT '',
		added_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (kind, value)
	)`); err != nil {
		return err
	}

	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS scan_runs (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		binary     TEXT NOT NULL,
		args       TEXT NOT NULL DEFAULT '',
		target     TEXT NOT NULL DEFAULT '',
		status     TEXT NOT NULL,              -- ran | denied | error
		denied     TEXT NOT NULL DEFAULT '',
		exit_code  INTEGER NOT NULL DEFAULT 0,
		stdout     TEXT NOT NULL DEFAULT '',
		stderr     TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	return s.ensureScanFindingsSchema()
}

type ScanRun struct {
	ID        int64  `json:"id"`
	Binary    string `json:"binary"`
	Args      string `json:"args"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	Denied    string `json:"denied"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) AddScanRun(r ScanRun) (int64, error) {
	cap64 := func(x string) string {
		if len(x) > 64<<10 {
			return x[:64<<10] + "…[truncated]"
		}
		return x
	}
	res, err := s.db.Exec(
		`INSERT INTO scan_runs(binary,args,target,status,denied,exit_code,stdout,stderr) VALUES(?,?,?,?,?,?,?,?)`,
		r.Binary, r.Args, r.Target, r.Status, r.Denied, r.ExitCode, cap64(r.Stdout), cap64(r.Stderr))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListScanRuns(limit int) ([]ScanRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id,binary,args,target,status,denied,exit_code,stdout,stderr,created_at FROM scan_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScanRun{}
	for rows.Next() {
		var r ScanRun
		if err := rows.Scan(&r.ID, &r.Binary, &r.Args, &r.Target, &r.Status, &r.Denied, &r.ExitCode, &r.Stdout, &r.Stderr, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListAllowlist(kind string) ([]AllowEntry, error) {
	var rows *sql.Rows
	var err error
	if kind == "" {
		rows, err = s.db.Query(`SELECT kind,value,note,added_at FROM scan_allowlist ORDER BY kind,value`)
	} else {
		rows, err = s.db.Query(`SELECT kind,value,note,added_at FROM scan_allowlist WHERE kind=? ORDER BY value`, kind)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AllowEntry{}
	for rows.Next() {
		var e AllowEntry
		if err := rows.Scan(&e.Kind, &e.Value, &e.Note, &e.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) AddAllowlist(kind, value, note string) error {
	kind = strings.TrimSpace(strings.ToLower(kind))
	value = strings.TrimSpace(value)
	if !validAllowKind(kind) {
		return fmt.Errorf("kind invalid (exec|target): %q", kind)
	}
	if value == "" {
		return fmt.Errorf("value kosong")
	}
	_, err := s.db.Exec(
		`INSERT INTO scan_allowlist(kind,value,note) VALUES(?,?,?)
		 ON CONFLICT(kind,value) DO UPDATE SET note=excluded.note`,
		kind, value, strings.TrimSpace(note))
	return err
}

func (s *Store) RemoveAllowlist(kind, value string) error {
	_, err := s.db.Exec(`DELETE FROM scan_allowlist WHERE kind=? AND value=?`,
		strings.TrimSpace(strings.ToLower(kind)), strings.TrimSpace(value))
	return err
}

func (s *Store) IsAllowed(kind, value string) (bool, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false, nil
	}
	entries, err := s.ListAllowlist(kind)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		ev := strings.ToLower(e.Value)
		if ev == value {
			return true, nil
		}
		if kind == "target" && strings.HasPrefix(ev, "*.") {
			suffix := ev[1:]
			if strings.HasSuffix(value, suffix) {
				return true, nil
			}
		}
	}
	return false, nil
}
