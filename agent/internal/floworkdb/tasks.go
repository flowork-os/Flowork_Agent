// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package floworkdb

import (
	"database/sql"
	"fmt"
)

type TaskCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	TriggerHint string `json:"trigger_hint"`
	Synthesizer string `json:"synthesizer"`

	SynthDirective string `json:"synth_directive"`

	WorkerDirective string      `json:"worker_directive"`
	Enabled         bool        `json:"enabled"`
	Crew            []TaskAgent `json:"crew,omitempty"`
}

type TaskAgent struct {
	AgentID   string `json:"agent_id"`
	RoleLabel string `json:"role_label"`
	OrderIdx  int    `json:"order_idx"`
	Mode      string `json:"mode"`
	Optional  bool   `json:"optional"`
}

type TaskRun struct {
	ID          int64      `json:"id"`
	CategoryID  string     `json:"category_id"`
	InputText   string     `json:"input_text"`
	Status      string     `json:"status"`
	RequestedBy string     `json:"requested_by"`
	Summary     string     `json:"summary"`
	StartedAt   string     `json:"started_at"`
	FinishedAt  string     `json:"finished_at"`
	NotifyChat  string     `json:"notify_chat,omitempty"`
	Steps       []TaskStep `json:"steps,omitempty"`
}

type TaskStep struct {
	ID        int64  `json:"id"`
	RunID     int64  `json:"run_id"`
	AgentID   string `json:"agent_id"`
	RoleLabel string `json:"role_label"`
	OrderIdx  int    `json:"order_idx"`
	Status    string `json:"status"`
	OutputRef string `json:"output_ref"`
	Err       string `json:"err"`
	MS        int64  `json:"ms"`
}

func (s *Store) EnsureTaskSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS task_categories (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL DEFAULT '',
			icon         TEXT NOT NULL DEFAULT '',
			trigger_hint TEXT NOT NULL DEFAULT '',
			synthesizer  TEXT NOT NULL DEFAULT '',
			enabled      INTEGER NOT NULL DEFAULT 1,
			created_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS task_agents (
			category_id TEXT NOT NULL,
			agent_id    TEXT NOT NULL,
			role_label  TEXT NOT NULL DEFAULT '',
			order_idx   INTEGER NOT NULL DEFAULT 0,
			mode        TEXT NOT NULL DEFAULT 'seq',
			optional    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (category_id, agent_id)
		)`,
		`CREATE TABLE IF NOT EXISTS task_runs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			category_id  TEXT NOT NULL,
			input_text   TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'running',
			requested_by TEXT NOT NULL DEFAULT '',
			summary      TEXT NOT NULL DEFAULT '',
			started_at   TEXT NOT NULL DEFAULT (datetime('now')),
			finished_at  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS task_run_steps (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id     INTEGER NOT NULL,
			agent_id   TEXT NOT NULL,
			role_label TEXT NOT NULL DEFAULT '',
			order_idx  INTEGER NOT NULL DEFAULT 0,
			status     TEXT NOT NULL DEFAULT 'pending',
			output_ref TEXT NOT NULL DEFAULT '',
			err        TEXT NOT NULL DEFAULT '',
			ms         INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_agents_cat ON task_agents(category_id, order_idx)`,
		`CREATE INDEX IF NOT EXISTS idx_task_runs_cat ON task_runs(category_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_task_steps_run ON task_run_steps(run_id, order_idx)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("task schema: %w", err)
		}
	}

	if !s.columnExists("task_categories", "synth_directive") {
		if _, err := s.db.Exec(
			`ALTER TABLE task_categories ADD COLUMN synth_directive TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("task schema migrate synth_directive: %w", err)
		}
	}

	if !s.columnExists("task_categories", "worker_directive") {
		if _, err := s.db.Exec(
			`ALTER TABLE task_categories ADD COLUMN worker_directive TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("task schema migrate worker_directive: %w", err)
		}
	}

	if !s.columnExists("task_runs", "notify_chat") {
		if _, err := s.db.Exec(
			`ALTER TABLE task_runs ADD COLUMN notify_chat TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("task schema migrate notify_chat: %w", err)
		}
	}
	return nil
}

func (s *Store) columnExists(table, col string) bool {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && name == col {
			return true
		}
	}
	return false
}

func (s *Store) UpsertCategory(c TaskCategory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := 0
	if c.Enabled {
		en = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO task_categories(id,name,icon,trigger_hint,synthesizer,synth_directive,worker_directive,enabled)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, icon=excluded.icon,
		   trigger_hint=excluded.trigger_hint, synthesizer=excluded.synthesizer,
		   synth_directive=excluded.synth_directive, worker_directive=excluded.worker_directive,
		   enabled=excluded.enabled`,
		c.ID, c.Name, c.Icon, c.TriggerHint, c.Synthesizer, c.SynthDirective, c.WorkerDirective, en)
	return err
}

func (s *Store) DeleteCategory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM task_agents WHERE category_id=?`, id); err != nil {
		return err
	}

	_, _ = s.db.Exec(`DELETE FROM trigger_rules WHERE target=?`, id)
	_, err := s.db.Exec(`DELETE FROM task_categories WHERE id=?`, id)
	return err
}

func (s *Store) CategoryIDs() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM task_categories`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func (s *Store) SetCrew(categoryID string, crew []TaskAgent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM task_agents WHERE category_id=?`, categoryID); err != nil {
		return err
	}
	for i, a := range crew {
		opt := 0
		if a.Optional {
			opt = 1
		}
		mode := a.Mode
		if mode == "" {
			mode = "seq"
		}
		if _, err := tx.Exec(
			`INSERT INTO task_agents(category_id,agent_id,role_label,order_idx,mode,optional)
			 VALUES(?,?,?,?,?,?)`,
			categoryID, a.AgentID, a.RoleLabel, i, mode, opt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetCategory(id string) (*TaskCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := TaskCategory{}
	var en int
	err := s.db.QueryRow(
		`SELECT id,name,icon,trigger_hint,synthesizer,synth_directive,worker_directive,enabled FROM task_categories WHERE id=?`, id,
	).Scan(&c.ID, &c.Name, &c.Icon, &c.TriggerHint, &c.Synthesizer, &c.SynthDirective, &c.WorkerDirective, &en)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = en == 1
	crew, err := s.crewLocked(id)
	if err != nil {
		return nil, err
	}
	c.Crew = crew
	return &c, nil
}

func (s *Store) crewLocked(categoryID string) ([]TaskAgent, error) {
	rows, err := s.db.Query(
		`SELECT agent_id,role_label,order_idx,mode,optional FROM task_agents
		 WHERE category_id=? ORDER BY order_idx`, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskAgent
	for rows.Next() {
		var a TaskAgent
		var opt int
		if err := rows.Scan(&a.AgentID, &a.RoleLabel, &a.OrderIdx, &a.Mode, &opt); err != nil {
			return nil, err
		}
		a.Optional = opt == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListCategories() ([]TaskCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT id,name,icon,trigger_hint,synthesizer,synth_directive,worker_directive,enabled FROM task_categories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskCategory
	for rows.Next() {
		var c TaskCategory
		var en int
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.TriggerHint, &c.Synthesizer, &c.SynthDirective, &c.WorkerDirective, &en); err != nil {
			return nil, err
		}
		c.Enabled = en == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

type CategoryRunStat struct {
	CategoryID  string `json:"category_id"`
	Done        int    `json:"done"`
	Error       int    `json:"error"`
	Interrupted int    `json:"interrupted"`
	Total       int    `json:"total"`
}

func (s *Store) CategoryRunStats() (map[string]CategoryRunStat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT category_id, status, COUNT(*) FROM task_runs GROUP BY category_id, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CategoryRunStat{}
	for rows.Next() {
		var cat, status string
		var n int
		if err := rows.Scan(&cat, &status, &n); err != nil {
			return nil, err
		}
		st := out[cat]
		st.CategoryID = cat
		st.Total += n
		switch status {
		case "done":
			st.Done += n
		case "error":
			st.Error += n
		case "interrupted":
			st.Interrupted += n
		}
		out[cat] = st
	}
	return out, rows.Err()
}

func (s *Store) CreateRun(categoryID, input, requestedBy, notifyChat string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO task_runs(category_id,input_text,status,requested_by,notify_chat) VALUES(?,?,'running',?,?)`,
		categoryID, input, requestedBy, notifyChat)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) StartStep(runID int64, agentID, role string, idx int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO task_run_steps(run_id,agent_id,role_label,order_idx,status) VALUES(?,?,?,?,'running')`,
		runID, agentID, role, idx)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishStep(stepID int64, status, outputRef, errStr string, ms int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE task_run_steps SET status=?, output_ref=?, err=?, ms=? WHERE id=?`,
		status, outputRef, errStr, ms, stepID)
	return err
}

func (s *Store) FinishRun(runID int64, status, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE task_runs SET status=?, summary=?, finished_at=datetime('now') WHERE id=?`,
		status, summary, runID)
	return err
}

func (s *Store) ListRuns(categoryID string, limit int) ([]TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT id,category_id,input_text,status,requested_by,summary,started_at,finished_at
		 FROM task_runs WHERE category_id=? ORDER BY id DESC LIMIT ?`, categoryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskRun
	for rows.Next() {
		var r TaskRun
		if err := rows.Scan(&r.ID, &r.CategoryID, &r.InputText, &r.Status, &r.RequestedBy,
			&r.Summary, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRun(runID int64) (*TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := TaskRun{}
	err := s.db.QueryRow(
		`SELECT id,category_id,input_text,status,requested_by,summary,started_at,finished_at
		 FROM task_runs WHERE id=?`, runID).Scan(
		&r.ID, &r.CategoryID, &r.InputText, &r.Status, &r.RequestedBy, &r.Summary, &r.StartedAt, &r.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id,run_id,agent_id,role_label,order_idx,status,output_ref,err,ms
		 FROM task_run_steps WHERE run_id=? ORDER BY order_idx, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st TaskStep
		if err := rows.Scan(&st.ID, &st.RunID, &st.AgentID, &st.RoleLabel, &st.OrderIdx,
			&st.Status, &st.OutputRef, &st.Err, &st.MS); err != nil {
			return nil, err
		}
		r.Steps = append(r.Steps, st)
	}
	return &r, rows.Err()
}

func (s *Store) MarkRunningInterrupted() ([]TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT id, category_id, input_text, COALESCE(notify_chat,'')
		 FROM task_runs WHERE status='running'`)
	if err != nil {
		return nil, err
	}
	var orphans []TaskRun
	for rows.Next() {
		var r TaskRun
		if err := rows.Scan(&r.ID, &r.CategoryID, &r.InputText, &r.NotifyChat); err != nil {
			rows.Close()
			return nil, err
		}
		r.Status = "interrupted"
		orphans = append(orphans, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(orphans) > 0 {
		if _, err := s.db.Exec(
			`UPDATE task_runs SET status='interrupted', finished_at=datetime('now')
			 WHERE status='running'`); err != nil {
			return orphans, err
		}
	}
	return orphans, nil
}
