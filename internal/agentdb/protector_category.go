// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Plug-in kolom category protector_rules (lazy ALTER idempotent via
//   pragma check). Ngga sentuh protector.go locked. E2E verified.
//
// protector_category.go — plug-in extension untuk protector_rules: kolom
// `category` (label UI: secrets/core/doktrin/entry/docs/config/custom).
//
// File terpisah (nano-modular) supaya protector.go yang LOCKED ngga disentuh.
// Lazy ALTER ADD COLUMN (idempotent) — pola ensure-schema lazy yang sama.

package agentdb

import "strings"

// ProtectorRuleCat = ProtectorRule + category label.
type ProtectorRuleCat struct {
	ProtectorRule
	Category string `json:"category"`
}

// ensureProtectorCategory — lazy ALTER ADD COLUMN category (idempotent).
// Dipanggil dari dalam method yang sudah pegang s.mu.
func (s *Store) ensureProtectorCategory() error {
	if err := s.ensureProtectorSchema(); err != nil {
		return err
	}
	// Cek kolom sudah ada via pragma (anti error "duplicate column").
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

// AddProtectorRuleCat — INSERT custom rule dengan category. rule_type tetap
// real (mis. "file_path") supaya interceptor enforcement jalan; category cuma
// label grouping UI. Reject source hardcoded (sama seperti AddProtectorRule).
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

// ListProtectorRulesCat — custom rows + category.
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

// FindProtectorRuleIDByPattern — cari id custom rule by pattern (untuk
// toggle/remove by path dari GUI). Return 0 kalau ngga ketemu.
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
