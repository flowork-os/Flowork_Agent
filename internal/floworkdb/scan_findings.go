// scan_findings.go — FINDINGS terstruktur hasil parse output scan tool (immune P2.2b + C).
//
// Output mentah scan tool (nmap/nuclei/trivy) DI-PARSE deterministik (NO LLM,
// prinsip #1 "agent BODOH, engine PINTER") → finding terstruktur:
//   severity · CWE · CVE · CVSS · component · evidence.
//
// Category = tujuan laporan (mirror tabel brain router, buat jembatan lintas-repo):
//   "immune"  → defensif (scan diri sendiri / supply-chain) = immune_system.
//   "pentest" → ofensif (HackerOne, scope authorized)        = pentest_karma.
//
// Owner-level (flowork.db) — sejajar scan_runs. Tiap finding nempel ke run_id
// (FK ke scan_runs) → jejak audit utuh. `verified` = slot reproducible_ok
// (prinsip #6 verifier: vuln ga "real" sebelum dikonfirmasi ulang). Default 0.

package floworkdb

import "fmt"

// ScanFinding — 1 temuan terstruktur dari parse output scan tool.
type ScanFinding struct {
	ID        int64   `json:"id"`
	RunID     int64   `json:"run_id"`
	Tool      string  `json:"tool"`      // nmap | nuclei | trivy
	Target    string  `json:"target"`    // host/url/path yang di-scan
	Category  string  `json:"category"`  // immune (defensif) | pentest (ofensif)
	Severity  string  `json:"severity"`  // info | low | medium | high | critical
	Title     string  `json:"title"`     // ringkasan temuan
	CWE       string  `json:"cwe"`       // mis. CWE-89 (opsional)
	CVE       string  `json:"cve"`       // mis. CVE-2019-14234 (opsional)
	CVSS      float64 `json:"cvss"`      // 0..10 (opsional)
	Component string  `json:"component"` // pkg@ver / port-service / template
	Evidence  string  `json:"evidence"`  // matched-at / lokasi / detail
	Verified  int     `json:"verified"`  // reproducible_ok: 0=belum, 1=terverifikasi
	CreatedAt string  `json:"created_at"`
}

// ensureScanFindingsSchema — tabel scan_findings (idempotent). Dipanggil dari
// EnsureScanSchema (boot).
func (s *Store) ensureScanFindingsSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS scan_findings (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id     INTEGER NOT NULL,
		tool       TEXT NOT NULL DEFAULT '',
		target     TEXT NOT NULL DEFAULT '',
		category   TEXT NOT NULL DEFAULT 'immune',  -- immune | pentest
		severity   TEXT NOT NULL DEFAULT 'info',    -- info|low|medium|high|critical
		title      TEXT NOT NULL,
		cwe        TEXT NOT NULL DEFAULT '',
		cve        TEXT NOT NULL DEFAULT '',
		cvss       REAL NOT NULL DEFAULT 0,
		component  TEXT NOT NULL DEFAULT '',
		evidence   TEXT NOT NULL DEFAULT '',
		verified   INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_scan_findings_run ON scan_findings(run_id)`)
	return nil
}

// AddScanFindings — simpan batch finding 1 run. Return jumlah ke-insert.
// Title kosong di-skip (defensif). Severity di-cap apa adanya (parser yang normalisasi).
func (s *Store) AddScanFindings(runID int64, fs []ScanFinding) (int, error) {
	if len(fs) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT INTO scan_findings
		(run_id,tool,target,category,severity,title,cwe,cve,cvss,component,evidence,verified)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	n := 0
	cap1k := func(x string) string {
		if len(x) > 1024 {
			return x[:1024] + "…"
		}
		return x
	}
	for _, f := range fs {
		if f.Title == "" {
			continue
		}
		if _, err := stmt.Exec(runID, f.Tool, cap1k(f.Target), f.Category, f.Severity,
			cap1k(f.Title), f.CWE, f.CVE, f.CVSS, cap1k(f.Component), cap1k(f.Evidence), f.Verified); err != nil {
			_ = tx.Rollback()
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// sevRank — urut keparahan buat ORDER BY (critical paling atas).
const sevRankSQL = `CASE severity WHEN 'critical' THEN 5 WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 ELSE 1 END`

func (s *Store) scanFindingRows(query string, args ...any) ([]ScanFinding, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScanFinding{}
	for rows.Next() {
		var f ScanFinding
		if err := rows.Scan(&f.ID, &f.RunID, &f.Tool, &f.Target, &f.Category, &f.Severity,
			&f.Title, &f.CWE, &f.CVE, &f.CVSS, &f.Component, &f.Evidence, &f.Verified, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

const scanFindingCols = `id,run_id,tool,target,category,severity,title,cwe,cve,cvss,component,evidence,verified,created_at`

// ListScanFindings — N finding terakhir, urut severity lalu waktu (buat laporan/GUI).
func (s *Store) ListScanFindings(limit int) ([]ScanFinding, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.scanFindingRows(
		`SELECT `+scanFindingCols+` FROM scan_findings ORDER BY `+sevRankSQL+` DESC, id DESC LIMIT ?`, limit)
}

// ListScanFindingsByRun — finding 1 run (urut severity).
func (s *Store) ListScanFindingsByRun(runID int64) ([]ScanFinding, error) {
	return s.scanFindingRows(
		`SELECT `+scanFindingCols+` FROM scan_findings WHERE run_id=? ORDER BY `+sevRankSQL+` DESC, id DESC`, runID)
}

// GetScanFinding — 1 finding by id (buat derive query triage RAG).
func (s *Store) GetScanFinding(id int64) (ScanFinding, error) {
	rows, err := s.scanFindingRows(`SELECT `+scanFindingCols+` FROM scan_findings WHERE id=?`, id)
	if err != nil {
		return ScanFinding{}, err
	}
	if len(rows) == 0 {
		return ScanFinding{}, fmt.Errorf("finding %d not found", id)
	}
	return rows[0], nil
}

// MarkFindingVerified — set slot reproducible_ok (verifier confirm). Prinsip #6.
func (s *Store) MarkFindingVerified(id int64, ok bool) error {
	v := 0
	if ok {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE scan_findings SET verified=? WHERE id=?`, v, id)
	return err
}

// CountFindingsBySeverity — ringkasan per severity (buat badge dashboard).
func (s *Store) CountFindingsBySeverity() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT severity, COUNT(*) FROM scan_findings GROUP BY severity`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var sev string
		var c int
		if err := rows.Scan(&sev, &c); err != nil {
			return nil, err
		}
		out[sev] = c
	}
	return out, rows.Err()
}
