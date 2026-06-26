// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import "strings"

type ProtectorRuleCat struct {
	ProtectorRule
	Category string `json:"category"`
}

func (s *Store) ensureProtectorCategory() error {
	if err := s.ensureProtectorSchema(); err != nil {
		return err
	}

	rows, err := s.db.Query(`PRAGMA table_info(protector_rules)`)
	if err != nil {
		return err
	}
	has := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if serr := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); serr != nil {
			rows.Close()
			return serr
		}
		if name == "category" {
			has = true
		}
	}
	rows.Close()
	if !has {
		if _, err := s.db.Exec(`ALTER TABLE protector_rules ADD COLUMN category TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AddProtectorRuleCat(ruleType, pattern, action, category string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorCategory(); err != nil {
		return 0, err
	}
	if ruleType == "" {
		ruleType = "file_path"
	}
	if action == "" {
		action = "block"
	}
	res, err := s.db.Exec(
		`INSERT INTO protector_rules (rule_type, pattern, action, source, enabled, category)
		 VALUES (?, ?, ?, 'custom', 1, ?)
		 ON CONFLICT(rule_type, pattern) DO UPDATE SET category=excluded.category, action=excluded.action, enabled=1`,
		ruleType, pattern, action, category,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListProtectorRulesCat() ([]ProtectorRuleCat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorCategory(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, rule_type, pattern, action, source, enabled, created_at, COALESCE(category,'')
		 FROM protector_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProtectorRuleCat{}
	for rows.Next() {
		var r ProtectorRuleCat
		var enabled int
		if serr := rows.Scan(&r.ID, &r.RuleType, &r.Pattern, &r.Action,
			&r.Source, &enabled, &r.CreatedAt, &r.Category); serr != nil {
			return nil, serr
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) FindProtectorRuleIDByPattern(pattern string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return 0, false
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM protector_rules WHERE pattern = ? ORDER BY id LIMIT 1`, strings.TrimSpace(pattern)).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}
