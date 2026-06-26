// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

package agentdb

import (
	"time"
)

type ScannerRun struct {
	ID            int64  `json:"id"`
	ScanType      string `json:"scan_type"`
	TargetPath    string `json:"target_path"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	TotalFindings int    `json:"total_findings"`
	CriticalCount int    `json:"critical_count"`
	Status        string `json:"status"`
}

type ScannerFinding struct {
	ID          int64  `json:"id"`
	RunID       int64  `json:"run_id"`
	Auditor     string `json:"auditor"`
	Severity    string `json:"severity"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Message     string `json:"message"`
	Snippet     string `json:"snippet"`
	Remediation string `json:"remediation"`
}

func (s *Store) ensureScannerSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS scanner_runs (
		  id             INTEGER PRIMARY KEY AUTOINCREMENT,
		  scan_type      TEXT NOT NULL,
		  target_path    TEXT NOT NULL,
		  started_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  finished_at    TEXT,
		  total_findings INTEGER NOT NULL DEFAULT 0,
		  critical_count INTEGER NOT NULL DEFAULT 0,
		  status         TEXT NOT NULL DEFAULT 'pending'
		);
		CREATE TABLE IF NOT EXISTS scanner_findings (
		  id          INTEGER PRIMARY KEY AUTOINCREMENT,
		  run_id      INTEGER NOT NULL,
		  auditor     TEXT NOT NULL,
		  severity    TEXT NOT NULL,
		  file_path   TEXT NOT NULL,
		  line_number INTEGER NOT NULL DEFAULT 0,
		  message     TEXT NOT NULL,
		  snippet     TEXT NOT NULL DEFAULT '',
		  remediation TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_findings_severity ON scanner_findings(severity);
		CREATE INDEX IF NOT EXISTS idx_findings_run ON scanner_findings(run_id);
		CREATE INDEX IF NOT EXISTS idx_scanner_runs_started ON scanner_runs(started_at DESC);
	`)
	return err
}

func (s *Store) InsertScannerRun(scanType, targetPath string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureScannerSchema(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO scanner_runs (scan_type, target_path, started_at, status)
		 VALUES (?, ?, ?, 'pending')`,
		scanType, targetPath, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishScannerRun(runID int64, total, critical int, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureScannerSchema(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE scanner_runs SET
		   finished_at = ?, total_findings = ?, critical_count = ?, status = ?
		 WHERE id = ?`,
		now, total, critical, status, runID,
	)
	return err
}

func (s *Store) InsertScannerFindings(runID int64, findings []ScannerFinding) error {
	if len(findings) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureScannerSchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT INTO scanner_findings
		   (run_id, auditor, severity, file_path, line_number, message, snippet, remediation)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, f := range findings {
		if _, err := stmt.Exec(runID, f.Auditor, f.Severity, f.FilePath,
			f.LineNumber, f.Message, f.Snippet, f.Remediation); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListScannerRuns(limit int) ([]ScannerRun, error) {
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureScannerSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, scan_type, target_path, started_at,
		        COALESCE(finished_at, ''), total_findings, critical_count, status
		 FROM scanner_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScannerRun{}
	for rows.Next() {
		var r ScannerRun
		if serr := rows.Scan(&r.ID, &r.ScanType, &r.TargetPath, &r.StartedAt,
			&r.FinishedAt, &r.TotalFindings, &r.CriticalCount, &r.Status); serr != nil {
			return nil, serr
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListScannerFindings(runID int64) ([]ScannerFinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureScannerSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, run_id, auditor, severity, file_path, line_number,
		        message, snippet, remediation
		 FROM scanner_findings WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScannerFinding{}
	for rows.Next() {
		var f ScannerFinding
		if serr := rows.Scan(&f.ID, &f.RunID, &f.Auditor, &f.Severity, &f.FilePath,
			&f.LineNumber, &f.Message, &f.Snippet, &f.Remediation); serr != nil {
			return nil, serr
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
