// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"sort"
	"strings"
	"time"
)

func (s *Store) ensureSkillCols() {
	for _, q := range []string{
		`ALTER TABLE skills ADD COLUMN created_at  TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE skills ADD COLUMN last_used   TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE skills ADD COLUMN usage_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE skills ADD COLUMN archived    INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = s.db.Exec(q)
	}
	_, _ = s.db.Exec(`UPDATE skills SET created_at=datetime('now') WHERE created_at=''`)
}

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

type SkillCurateReport struct {
	Active       int      `json:"active"`
	Consolidated []string `json:"consolidated"`
	Stale        []string `json:"stale"`
	TopGraded    []string `json:"top_graded"`
}

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

func (s *Store) BumpSkillUsage(id string) {
	s.ensureSkillCols()
	_, _ = s.db.Exec(
		`UPDATE skills SET usage_count=usage_count+1, last_used=datetime('now') WHERE id=?`, id)
}

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
				return group[i].UsageCount > group[j].UsageCount
			}
			return group[i].CreatedAt < group[j].CreatedAt
		})
		for _, dup := range group[1:] {
			archive(dup.ID, "dup")
			archived[dup.ID] = true
		}
	}

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
