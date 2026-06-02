// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev). Locked: 2026-06-02.
// Reason: FASE 8 Curator skill lifecycle. E2E verified: consolidate dup (keep
//   usage tertinggi), stale→archive (idle 90d/umur 30d usage 0), grade. Soft-
//   archive (recoverable). Extend (auto-create) → tambah method baru.
//
// skills_curate.go — FASE 8: Curator skill lifecycle (per-agent, isolated).
//
// Skill numpuk (apalagi nanti ada auto-create) → perlu di-curate biar prompt ga
// keracunan skill basi/duplikat. Curator:
//   - GRADE      : skor per-skill (usage_count + recency).
//   - CONSOLIDATE: skill instruksi IDENTIK → simpen 1 (usage tertinggi), arsip sisanya.
//   - STALE→ARSIP: skill ga kepake lama (idle > N hari) atau lama-ga-pernah-kepake
//                  (umur > M hari, usage 0) → archived=1 (soft, bisa balik).
//
// Soft-archive (archived=1) BUKAN delete — recoverable. Skill archived ga
// di-inject ke prompt (anti over-prompt) tapi masih ke-simpen.

package agentdb

import (
	"sort"
	"strings"
	"time"
)

// ensureSkillCols — tambah kolom lifecycle ke `skills` (idempotent). ALTER ADD
// COLUMN error kalau udah ada → di-ignore. Backfill created_at row lama.
func (s *Store) ensureSkillCols() {
	for _, q := range []string{
		`ALTER TABLE skills ADD COLUMN created_at  TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE skills ADD COLUMN last_used   TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE skills ADD COLUMN usage_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE skills ADD COLUMN archived    INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = s.db.Exec(q) // ignore "duplicate column"
	}
	_, _ = s.db.Exec(`UPDATE skills SET created_at=datetime('now') WHERE created_at=''`)
}

// SkillRow — skill + metadata lifecycle + grade.
type SkillRow struct {
	ID           string `json:"id"`
	Trigger      string `json:"trigger"`
	Instructions string `json:"instructions"`
	CreatedAt    string `json:"created_at"`
	LastUsed     string `json:"last_used"`
	UsageCount   int    `json:"usage_count"`
	Archived     bool   `json:"archived"`
	Grade        int    `json:"grade"`
}

// SkillCurateReport — hasil 1 sapuan curator.
type SkillCurateReport struct {
	Active       int      `json:"active"`
	Consolidated []string `json:"consolidated"` // id di-arsip krn duplikat
	Stale        []string `json:"stale"`        // id di-arsip krn basi/idle
	TopGraded    []string `json:"top_graded"`   // id skill skor tertinggi (≤5)
}

// AddSkill — insert skill (atau update kalau id sama). created_at = now.
func (s *Store) AddSkill(id, trigger, instructions string, orderIdx int) error {
	s.ensureSkillCols()
	_, err := s.db.Exec(
		`INSERT INTO skills(id,trigger,instructions,order_idx,created_at)
		 VALUES(?,?,?,?,datetime('now'))
		 ON CONFLICT(id) DO UPDATE SET trigger=excluded.trigger,
		   instructions=excluded.instructions, order_idx=excluded.order_idx`,
		id, trigger, instructions, orderIdx)
	return err
}

// BumpSkillUsage — catat skill kepake (usage_count++, last_used=now). Dipanggil
// pas skill di-surface ke agent (inject/search). Best-effort.
func (s *Store) BumpSkillUsage(id string) {
	s.ensureSkillCols()
	_, _ = s.db.Exec(
		`UPDATE skills SET usage_count=usage_count+1, last_used=datetime('now') WHERE id=?`, id)
}

// gradeSkill — skor: usage dominan + bonus recency. Dipakai sort + report.
func gradeSkill(usage int, lastUsed string, now time.Time) int {
	g := usage * 10
	if lastUsed != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", lastUsed); err == nil {
			days := now.Sub(t).Hours() / 24
			switch {
			case days < 7:
				g += 5
			case days < 30:
				g += 2
			}
		}
	}
	return g
}

// ListSkillsGraded — semua skill (+ grade). includeArchived=false → aktif doang.
func (s *Store) ListSkillsGraded(includeArchived bool) ([]SkillRow, error) {
	s.ensureSkillCols()
	q := `SELECT id,trigger,instructions,created_at,last_used,usage_count,archived FROM skills`
	if !includeArchived {
		q += ` WHERE archived=0`
	}
	q += ` ORDER BY order_idx, id`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	var out []SkillRow
	for rows.Next() {
		var r SkillRow
		var arch int
		if err := rows.Scan(&r.ID, &r.Trigger, &r.Instructions, &r.CreatedAt,
			&r.LastUsed, &r.UsageCount, &arch); err != nil {
			return nil, err
		}
		r.Archived = arch == 1
		r.Grade = gradeSkill(r.UsageCount, r.LastUsed, now)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CurateSkills — 1 sapuan: consolidate dup + arsip stale + grade. now di-pass
// caller (testable). idleDays = idle > ini → arsip; ageDays = umur > ini & usage 0
// → arsip.
func (s *Store) CurateSkills(now time.Time, idleDays, ageDays int) (SkillCurateReport, error) {
	s.ensureSkillCols()
	rep := SkillCurateReport{}
	active, err := s.ListSkillsGraded(false)
	if err != nil {
		return rep, err
	}

	archive := func(id, reason string) {
		_, _ = s.db.Exec(`UPDATE skills SET archived=1 WHERE id=?`, id)
		if reason == "dup" {
			rep.Consolidated = append(rep.Consolidated, id)
		} else {
			rep.Stale = append(rep.Stale, id)
		}
	}
	archived := map[string]bool{}

	// 1) CONSOLIDATE: group by instruksi ternormalisasi. Simpen usage tertinggi
	//    (tie → created_at paling tua), arsip sisanya.
	byInstr := map[string][]SkillRow{}
	for _, sk := range active {
		key := strings.ToLower(strings.Join(strings.Fields(sk.Instructions), " "))
		if key == "" {
			continue
		}
		byInstr[key] = append(byInstr[key], sk)
	}
	for _, group := range byInstr {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].UsageCount != group[j].UsageCount {
				return group[i].UsageCount > group[j].UsageCount // usage tinggi di depan
			}
			return group[i].CreatedAt < group[j].CreatedAt // lebih tua di depan
		})
		for _, dup := range group[1:] {
			archive(dup.ID, "dup")
			archived[dup.ID] = true
		}
	}

	// 2) STALE→ARSIP: idle lama, atau umur tua tapi ga pernah kepake.
	for _, sk := range active {
		if archived[sk.ID] {
			continue
		}
		if sk.LastUsed != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", sk.LastUsed); err == nil &&
				now.Sub(t).Hours()/24 > float64(idleDays) {
				archive(sk.ID, "stale")
				archived[sk.ID] = true
				continue
			}
		}
		if sk.UsageCount == 0 && sk.CreatedAt != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", sk.CreatedAt); err == nil &&
				now.Sub(t).Hours()/24 > float64(ageDays) {
				archive(sk.ID, "stale")
				archived[sk.ID] = true
			}
		}
	}

	// 3) GRADE: ranking skill yang masih aktif.
	remain, _ := s.ListSkillsGraded(false)
	sort.Slice(remain, func(i, j int) bool { return remain[i].Grade > remain[j].Grade })
	rep.Active = len(remain)
	for i, sk := range remain {
		if i >= 5 {
			break
		}
		rep.TopGraded = append(rep.TopGraded, sk.ID)
	}
	return rep, nil
}
