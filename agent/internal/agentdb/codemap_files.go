// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import "encoding/json"

type CodemapFile struct {
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	FileType        string   `json:"file_type"`
	LineCount       int      `json:"line_count"`
	Layer           string   `json:"layer"`
	HasTests        bool     `json:"has_tests"`
	HasDocs         bool     `json:"has_docs"`
	HealthScore     int      `json:"health_score"`
	RecentlyTouched bool     `json:"recently_touched"`
	Issues          []string `json:"issues"`
}

type CodemapFileEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (s *Store) ensureCodemapFilesSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS codemap_files (
		  path             TEXT PRIMARY KEY,
		  name             TEXT NOT NULL DEFAULT '',
		  file_type        TEXT NOT NULL DEFAULT 'go',
		  line_count       INTEGER NOT NULL DEFAULT 0,
		  layer            TEXT NOT NULL DEFAULT '',
		  has_tests        INTEGER NOT NULL DEFAULT 0,
		  has_docs         INTEGER NOT NULL DEFAULT 0,
		  health_score     INTEGER NOT NULL DEFAULT 100,
		  recently_touched INTEGER NOT NULL DEFAULT 0,
		  issues_json      TEXT NOT NULL DEFAULT '[]',
		  indexed_at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS codemap_file_edges (
		  from_path TEXT NOT NULL,
		  to_path   TEXT NOT NULL,
		  PRIMARY KEY (from_path, to_path)
		);
	`)
	return err
}

func (s *Store) ReplaceCodemapFiles(files []CodemapFile, edges []CodemapFileEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureCodemapFilesSchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM codemap_files`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM codemap_file_edges`); err != nil {
		return err
	}
	for _, f := range files {
		b2i := func(b bool) int {
			if b {
				return 1
			}
			return 0
		}
		issues, _ := json.Marshal(f.Issues)
		if _, err := tx.Exec(
			`INSERT INTO codemap_files (path,name,file_type,line_count,layer,has_tests,has_docs,health_score,recently_touched,issues_json)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			f.Path, f.Name, f.FileType, f.LineCount, f.Layer, b2i(f.HasTests), b2i(f.HasDocs), f.HealthScore, b2i(f.RecentlyTouched), string(issues),
		); err != nil {
			return err
		}
	}
	for _, e := range edges {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO codemap_file_edges (from_path,to_path) VALUES (?,?)`,
			e.From, e.To,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListCodemapFiles() ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureCodemapFilesSchema(); err != nil {
		return nil, err
	}

	depCount := map[string]int{}
	erows, err := s.db.Query(`SELECT to_path, COUNT(*) FROM codemap_file_edges GROUP BY to_path`)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var p string
		var c int
		if serr := erows.Scan(&p, &c); serr != nil {
			erows.Close()
			return nil, serr
		}
		depCount[p] = c
	}
	erows.Close()

	rows, err := s.db.Query(`SELECT path,name,file_type,line_count,layer,has_tests,has_docs,health_score,recently_touched,issues_json FROM codemap_files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var path, name, ftype, layer, issuesJSON string
		var loc, health, hasTests, hasDocs, touched int
		if serr := rows.Scan(&path, &name, &ftype, &loc, &layer, &hasTests, &hasDocs, &health, &touched, &issuesJSON); serr != nil {
			return nil, serr
		}
		var issues []string
		_ = json.Unmarshal([]byte(issuesJSON), &issues)
		if issues == nil {
			issues = []string{}
		}
		out = append(out, map[string]any{
			"path": path, "name": name, "file_type": ftype, "line_count": loc,
			"layer": layer, "has_tests": hasTests != 0, "has_docs": hasDocs != 0,
			"health_score": health, "recently_touched": touched != 0,
			"issues": issues, "dependent_count": depCount[path],
		})
	}
	return out, rows.Err()
}

func (s *Store) ListCodemapFileEdges() ([]CodemapFileEdge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureCodemapFilesSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT from_path, to_path FROM codemap_file_edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CodemapFileEdge{}
	for rows.Next() {
		var e CodemapFileEdge
		if serr := rows.Scan(&e.From, &e.To); serr != nil {
			return nil, serr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
