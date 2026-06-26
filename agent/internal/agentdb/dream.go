// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DreamResult struct {
	MistakesScanned int    `json:"mistakes_scanned"`
	EurekasFormed   int    `json:"eurekas_formed"`
	EurekasTotal    int    `json:"eurekas_total"`
	LogPath         string `json:"log_path"`
}

func (s *Store) recurringMistakes(minHit int64, limit int) ([]Mistake, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT id, category, title, content, context_origin, tier, hit_count,
		        last_hit_at, created_at
		   FROM mistakes_local
		  WHERE deleted_at IS NULL AND hit_count >= ?
		  ORDER BY hit_count DESC
		  LIMIT ?`, minHit, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mistake{}
	for rows.Next() {
		var m Mistake
		if err := rows.Scan(&m.ID, &m.Category, &m.Title, &m.Content, &m.ContextOrigin,
			&m.Tier, &m.HitCount, &m.LastHitAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) RunDream(now time.Time) (DreamResult, error) {
	var res DreamResult
	mistakes, err := s.recurringMistakes(2, 50)
	if err != nil {
		return res, fmt.Errorf("dream scan: %w", err)
	}
	res.MistakesScanned = len(mistakes)
	if len(mistakes) == 0 {
		return res, nil
	}

	var lines []string
	for _, m := range mistakes {
		eureka := fmt.Sprintf(
			"EUREKA (pola berulang): \"%s\" kejadian %d kali. Pelajaran/solusi: %s",
			m.Title, m.HitCount, m.Content)
		id, added, aerr := s.AddBrainDrawer(eureka, "eureka", m.Category, "eureka", "dream")
		if aerr != nil {
			continue
		}
		res.EurekasTotal++
		if added {
			res.EurekasFormed++
		}
		mark := ""
		if !added {
			mark = " (udah ada)"
		}
		lines = append(lines, fmt.Sprintf("- [%dx] %s → %s  [drawer %s]%s",
			m.HitCount, m.Title, m.Content, id, mark))
	}

	res.LogPath = s.writeDreamLog(now, lines)
	return res, nil
}

func (s *Store) writeDreamLog(now time.Time, lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	dir := filepath.Join(filepath.Dir(s.Path), "dreams")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	date := now.UTC().Format("2006-01-02")
	path := filepath.Join(dir, date+".md")
	var b strings.Builder
	b.WriteString("# Dream — ")
	b.WriteString(now.UTC().Format(time.RFC3339))
	b.WriteString("\n\nEureka dari pola berulang (mistakes hit_count≥2):\n\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return ""
	}
	return path
}
