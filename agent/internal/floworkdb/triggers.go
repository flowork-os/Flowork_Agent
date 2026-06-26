// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/trigger-schedule.md

package floworkdb

import (
	"database/sql"
	"time"
)

type Trigger struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TypeID        string `json:"type_id"`
	Config        string `json:"config"`
	Target        string `json:"target"`
	TargetKind    string `json:"target_kind"`
	Prompt        string `json:"prompt"`
	Deliver       string `json:"deliver"`
	Enabled       bool   `json:"enabled"`
	State         string `json:"state"`
	WebhookSecret string `json:"webhook_secret"`
	LastFired     string `json:"last_fired"`
	LastStatus    string `json:"last_status"`
	CreatedAt     string `json:"created_at"`
}

type TriggerRun struct {
	ID          int64  `json:"id"`
	RuleID      string `json:"rule_id"`
	FiredAt     string `json:"fired_at"`
	FinishedAt  string `json:"finished_at"`
	Trigger     string `json:"trigger"`
	Status      string `json:"status"`
	PayloadJSON string `json:"payload_json"`
	ResultText  string `json:"result_text"`
	ErrorText   string `json:"error_text"`
}

func (s *Store) EnsureTriggerSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trigger_rules (
		  id             TEXT PRIMARY KEY,
		  name           TEXT NOT NULL,
		  type_id        TEXT NOT NULL,
		  config         TEXT NOT NULL DEFAULT '{}',
		  target         TEXT NOT NULL,
		  target_kind    TEXT NOT NULL DEFAULT 'agent',
		  prompt         TEXT NOT NULL,
		  deliver        TEXT NOT NULL DEFAULT 'telegram',
		  enabled        INTEGER NOT NULL DEFAULT 1,
		  state          TEXT NOT NULL DEFAULT '',
		  webhook_secret TEXT NOT NULL DEFAULT '',
		  last_fired     TEXT NOT NULL DEFAULT '',
		  last_status    TEXT NOT NULL DEFAULT '',
		  created_at     TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE IF NOT EXISTS trigger_fired_keys (
		  rule_id  TEXT NOT NULL,
		  key      TEXT NOT NULL,
		  fired_at TEXT NOT NULL DEFAULT (datetime('now')),
		  PRIMARY KEY (rule_id, key)
		);
		CREATE TABLE IF NOT EXISTS trigger_runs (
		  id           INTEGER PRIMARY KEY AUTOINCREMENT,
		  rule_id      TEXT NOT NULL,
		  fired_at     TEXT NOT NULL DEFAULT (datetime('now')),
		  finished_at  TEXT NOT NULL DEFAULT '',
		  trigger      TEXT NOT NULL DEFAULT 'poll',
		  status       TEXT NOT NULL DEFAULT 'running',
		  payload_json TEXT NOT NULL DEFAULT '{}',
		  result_text  TEXT NOT NULL DEFAULT '',
		  error_text   TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_trigger_runs_rid ON trigger_runs(rule_id, id DESC);
	`)
	return err
}

func scanTrigger(rows *sql.Rows) (Trigger, error) {
	var t Trigger
	var en int
	err := rows.Scan(&t.ID, &t.Name, &t.TypeID, &t.Config, &t.Target, &t.TargetKind,
		&t.Prompt, &t.Deliver, &en, &t.State, &t.WebhookSecret, &t.LastFired, &t.LastStatus, &t.CreatedAt)
	t.Enabled = en != 0
	return t, err
}

const triggerCols = `id,name,type_id,config,target,target_kind,prompt,deliver,enabled,state,webhook_secret,last_fired,last_status,created_at`

func (s *Store) ListTriggers() ([]Trigger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT ` + triggerCols + ` FROM trigger_rules ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trigger{}
	for rows.Next() {
		t, e := scanTrigger(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTrigger(id string) (*Trigger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT `+triggerCols+` FROM trigger_rules WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	t, e := scanTrigger(rows)
	if e != nil {
		return nil, e
	}
	return &t, nil
}

func (s *Store) UpsertTrigger(t Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := 0
	if t.Enabled {
		en = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO trigger_rules (id,name,type_id,config,target,target_kind,prompt,deliver,enabled,webhook_secret)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  name=excluded.name, type_id=excluded.type_id, config=excluded.config,
		  target=excluded.target, target_kind=excluded.target_kind, prompt=excluded.prompt,
		  deliver=excluded.deliver, enabled=excluded.enabled,
		  webhook_secret=CASE WHEN excluded.webhook_secret<>'' THEN excluded.webhook_secret ELSE trigger_rules.webhook_secret END,
		  -- ganti TIPE = reset state opaque: state tipe lama (mis. "menit" dari time)
		  -- kalau dibaca tipe baru (mis. file-watch) bisa salah-tafsir → fire massal.
		  state=CASE WHEN excluded.type_id<>trigger_rules.type_id THEN '' ELSE trigger_rules.state END`,
		t.ID, t.Name, t.TypeID, t.Config, t.Target, t.TargetKind, t.Prompt, t.Deliver, en, t.WebhookSecret)
	return err
}

func (s *Store) DeleteTrigger(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM trigger_rules WHERE id=?`,
		`DELETE FROM trigger_fired_keys WHERE rule_id=?`,
		`DELETE FROM trigger_runs WHERE rule_id=?`,
	} {
		if _, e := tx.Exec(q, id); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func (s *Store) SetTriggerEnabled(id string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := 0
	if on {
		en = 1
	}
	_, err := s.db.Exec(`UPDATE trigger_rules SET enabled=? WHERE id=?`, en, id)
	return err
}

func (s *Store) DisableTriggersByType(typeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE trigger_rules SET enabled=0, last_status='type_removed' WHERE type_id=?`, typeID)
	return err
}

func (s *Store) TouchTrigger(id, state, lastFired, lastStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE trigger_rules SET state=?, last_fired=?, last_status=? WHERE id=?`,
		state, lastFired, lastStatus, id)
	return err
}

func (s *Store) MarkTriggerFired(id, lastFired, lastStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE trigger_rules SET last_fired=?, last_status=? WHERE id=?`, lastFired, lastStatus, id)
	return err
}

func (s *Store) MarkTriggerKey(ruleID, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`INSERT OR IGNORE INTO trigger_fired_keys (rule_id,key) VALUES (?,?)`, ruleID, key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) SweepTriggerKeys(olderThanDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().UTC().AddDate(0, 0, -olderThanDays).Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(`DELETE FROM trigger_fired_keys WHERE fired_at < ?`, cutoff)
	return err
}

func (s *Store) InsertTriggerRun(ruleID, trigger, payloadJSON string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`INSERT INTO trigger_runs (rule_id,trigger,payload_json) VALUES (?,?,?)`,
		ruleID, trigger, payloadJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishTriggerRun(runID int64, status, result, errText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if len(result) > 8000 {
		result = result[:8000] + "…"
	}
	_, err := s.db.Exec(`UPDATE trigger_runs SET finished_at=?, status=?, result_text=?, error_text=? WHERE id=?`,
		now, status, result, errText, runID)
	return err
}

func (s *Store) ListTriggerRuns(ruleID string, limit int) ([]TriggerRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id,rule_id,fired_at,finished_at,trigger,status,payload_json,result_text,error_text
		FROM trigger_runs WHERE rule_id=? ORDER BY id DESC LIMIT ?`, ruleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TriggerRun{}
	for rows.Next() {
		var r TriggerRun
		if e := rows.Scan(&r.ID, &r.RuleID, &r.FiredAt, &r.FinishedAt, &r.Trigger, &r.Status,
			&r.PayloadJSON, &r.ResultText, &r.ErrorText); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
