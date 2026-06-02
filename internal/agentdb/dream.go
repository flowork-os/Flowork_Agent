// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B3 dream. Verified: recurring mistakes(hit>=2)->eureka drawer
//   (dedup), brain_search recall, idempotent, dream log. Host cron 12h shared-worker.
//   Extend -> file baru, JANGAN modify ini.
//
// dream.go — Roadmap 2 Fase B3: Dream (konsolidasi idle → eureka).
//
// Adaptasi dari worker/internal/dreamstate/dream.go: ekstrak POLA BERULANG
// (signal over noise: ≥2 occurrence) → sintesis EUREKA rule-based (NO LLM,
// hemat + deterministik). Di arsitektur kita, "≥2 occurrence" = mistakes_local
// dengan hit_count≥2 (pola yang keulang). Tiap pola → drawer brain mem_type=
// 'eureka' (recallable via brain_search) + log dreams/<date>.md (portable).
//
// Anti-boros (roadmap 1.5): dijalanin SATU host-cron buat semua agent (compute
// 1×), tulis ke state.db lokal masing-masing. Dedup via brain content_hash.

package agentdb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DreamResult — ringkasan 1 siklus dream.
type DreamResult struct {
	MistakesScanned int    `json:"mistakes_scanned"`
	EurekasFormed   int    `json:"eurekas_formed"` // drawer eureka BARU (dedup-aware)
	EurekasTotal    int    `json:"eurekas_total"`  // total pola jadi eureka (incl. yg udah ada)
	LogPath         string `json:"log_path"`
}

// recurringMistakes — mistake live dengan hit_count >= minHit (pola berulang).
// Locked read; caller (RunDream) ga pegang lock pas manggil ini.
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

// RunDream — 1 siklus konsolidasi. Baca pola berulang → eureka drawer + dream log.
// Idempotent (dedup brain). Aman dipanggil berulang (cron). NO lock di sini —
// sub-call (recurringMistakes/AddBrainDrawer) yang lock masing-masing.
func (s *Store) RunDream(now time.Time) (DreamResult, error) {
	var res DreamResult
	mistakes, err := s.recurringMistakes(2, 50)
	if err != nil {
		return res, fmt.Errorf("dream scan: %w", err)
	}
	res.MistakesScanned = len(mistakes)
	if len(mistakes) == 0 {
		return res, nil // ga ngimpi kalau ga ada pola
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

// writeDreamLog tulis ringkasan eureka ke <workspace>/dreams/<date>.md (portable,
// ikut folder agent). Best-effort — gagal nulis file ga bikin dream gagal.
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
