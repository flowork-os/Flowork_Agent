// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 29 zombie_findings + Section 32 mode_config (kv shortcut)
//   + Section 35 self_prompt schema. Lazy CREATE. Phase 2 (real zombie
//   heuristic via callgraph, mode handler integration, prompt versioning)
//   → tambah file baru.
//
// zombie_modes_prompt.go — Sections 29 + 32 + 35 phase 1 minimal schemas.

package agentdb

import (
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// Section 29: zombie_findings
// =============================================================================

type ZombieFinding struct {
	ID          int64  `json:"id"`
	FilePath    string `json:"file_path"`
	SymbolName  string `json:"symbol_name"`
	SymbolType  string `json:"symbol_type"` // 'file' | 'func' | 'type'
	Confidence  string `json:"confidence"`  // 'high' | 'medium' | 'low'
	Reason      string `json:"reason"`
	DetectedAt  string `json:"detected_at"`
	Acknowledged bool  `json:"acknowledged"`
}

// =============================================================================
// Section 35: self_prompt (per-warga self-contained prompt.md storage)
// =============================================================================

type SelfPrompt struct {
	ID        int64  `json:"id"`
	Slot      string `json:"slot"`       // 'system' | 'persona' | 'guideline' | 'task'
	Version   int    `json:"version"`
	Body      string `json:"body"`       // markdown content
	UpdatedAt string `json:"updated_at"`
	Notes     string `json:"notes"`
}

func (s *Store) ensureSec29Sec35Schema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS zombie_findings (
		  id          INTEGER PRIMARY KEY AUTOINCREMENT,
		  file_path   TEXT NOT NULL,
		  symbol_name TEXT NOT NULL DEFAULT '',
		  symbol_type TEXT NOT NULL DEFAULT 'file',
		  confidence  TEXT NOT NULL DEFAULT 'medium',
		  reason      TEXT NOT NULL DEFAULT '',
		  detected_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  acknowledged INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_zombie_file ON zombie_findings(file_path);
		CREATE INDEX IF NOT EXISTS idx_zombie_ack ON zombie_findings(acknowledged);

		CREATE TABLE IF NOT EXISTS self_prompt (
		  id         INTEGER PRIMARY KEY AUTOINCREMENT,
		  slot       TEXT NOT NULL,
		  version    INTEGER NOT NULL DEFAULT 1,
		  body       TEXT NOT NULL DEFAULT '',
		  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  notes      TEXT NOT NULL DEFAULT '',
		  UNIQUE(slot, version)
		);
		CREATE INDEX IF NOT EXISTS idx_self_prompt_slot ON self_prompt(slot, version DESC);
	`)
	return err
}

func (s *Store) AddZombieFinding(z ZombieFinding) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSec29Sec35Schema(); err != nil {
		return 0, err
	}
	if z.FilePath == "" {
		return 0, fmt.Errorf("file_path required")
	}
	if z.SymbolType == "" {
		z.SymbolType = "file"
	}
	if z.Confidence == "" {
		z.Confidence = "medium"
	}
	if z.DetectedAt == "" {
		z.DetectedAt = time.Now().UTC().Format(time.RFC3339)
	}
	ack := 0
	if z.Acknowledged {
		ack = 1
	}
	res, err := s.db.Exec(
		`INSERT INTO zombie_findings (file_path, symbol_name, symbol_type, confidence, reason, detected_at, acknowledged)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		z.FilePath, z.SymbolName, z.SymbolType, z.Confidence, z.Reason, z.DetectedAt, ack,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListZombieFindings(limit int) ([]ZombieFinding, error) {
	if limit <= 0 {
		limit = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSec29Sec35Schema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, file_path, symbol_name, symbol_type, confidence, reason, detected_at, acknowledged
		 FROM zombie_findings ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ZombieFinding{}
	for rows.Next() {
		var z ZombieFinding
		var ack int
		if serr := rows.Scan(&z.ID, &z.FilePath, &z.SymbolName, &z.SymbolType,
			&z.Confidence, &z.Reason, &z.DetectedAt, &ack); serr != nil {
			return nil, serr
		}
		z.Acknowledged = ack != 0
		out = append(out, z)
	}
	return out, rows.Err()
}

func (s *Store) AcknowledgeZombie(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSec29Sec35Schema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE zombie_findings SET acknowledged = 1 WHERE id = ?`, id)
	return err
}

// Section 35: prompt slot CRUD. Versioned (UNIQUE slot+version).
// SetSelfPrompt insert next version (caller pass version 0 → auto-increment).
func (s *Store) SetSelfPrompt(slot, body, notes string, version int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSec29Sec35Schema(); err != nil {
		return 0, err
	}
	slot = strings.TrimSpace(slot)
	if slot == "" {
		return 0, fmt.Errorf("slot required")
	}
	if len(body) > 64*1024 {
		return 0, fmt.Errorf("body too large (cap 64KB)")
	}
	if version <= 0 {
		// Auto-increment from MAX(version).
		var maxV int
		_ = s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM self_prompt WHERE slot = ?`, slot).Scan(&maxV)
		version = maxV + 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO self_prompt (slot, version, body, updated_at, notes)
		 VALUES (?, ?, ?, ?, ?)`,
		slot, version, body, now, notes,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetSelfPrompt(slot string, version int) (SelfPrompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSec29Sec35Schema(); err != nil {
		return SelfPrompt{}, err
	}
	if version <= 0 {
		// Latest version.
		var sp SelfPrompt
		err := s.db.QueryRow(
			`SELECT id, slot, version, body, updated_at, notes
			 FROM self_prompt WHERE slot = ? ORDER BY version DESC LIMIT 1`, slot,
		).Scan(&sp.ID, &sp.Slot, &sp.Version, &sp.Body, &sp.UpdatedAt, &sp.Notes)
		return sp, err
	}
	var sp SelfPrompt
	err := s.db.QueryRow(
		`SELECT id, slot, version, body, updated_at, notes
		 FROM self_prompt WHERE slot = ? AND version = ?`, slot, version,
	).Scan(&sp.ID, &sp.Slot, &sp.Version, &sp.Body, &sp.UpdatedAt, &sp.Notes)
	return sp, err
}

func (s *Store) ListSelfPromptSlots() ([]SelfPrompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSec29Sec35Schema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, slot, version, body, updated_at, notes
		 FROM self_prompt
		 WHERE version IN (SELECT MAX(version) FROM self_prompt GROUP BY slot)
		 ORDER BY slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SelfPrompt{}
	for rows.Next() {
		var sp SelfPrompt
		if serr := rows.Scan(&sp.ID, &sp.Slot, &sp.Version, &sp.Body, &sp.UpdatedAt, &sp.Notes); serr != nil {
			return nil, serr
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}
