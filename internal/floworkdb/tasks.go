// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-02
// Reason: FASE 5 — data model Category Task (owner-level). E2E verified:
//   seed SAHAM, run async DB-driven, timeline live persist, synth adaptif
//   partial-failure. Extend (kategori baru/kolom) → tambah migrasi, jaga
//   EnsureTaskSchema idempotent.
//
// tasks.go — FASE 5: data model Category Task (owner-level, flowork.db).
//
// Bikin definisi task (kategori + crew) bisa diatur owner dari GUI, BUKAN
// hardcoded di taskflow.go (Fase 4). + simpen run history + step status buat
// timeline live. Worker tetep kerja di state.db-nya sendiri (isolated); ini
// cuma DEFINISI + AUDIT run, bukan memori warga.
//
// Tabel (per doc/category_task_design.md §5):
//   task_categories(id, name, icon, trigger_hint, synthesizer, enabled, created_at)
//   task_agents(category_id, agent_id, role_label, order_idx, mode, optional)
//   task_runs(id, category_id, input_text, status, requested_by, summary, started_at, finished_at)
//   task_run_steps(id, run_id, agent_id, role_label, order_idx, status, output_ref, err, ms)

package floworkdb

import (
	"database/sql"
	"fmt"
)

// ── Types ────────────────────────────────────────────────────────────────────

type TaskCategory struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Icon        string     `json:"icon"`
	TriggerHint string     `json:"trigger_hint"`
	Synthesizer string     `json:"synthesizer"`
	Enabled     bool       `json:"enabled"`
	Crew        []TaskAgent `json:"crew,omitempty"`
}

type TaskAgent struct {
	AgentID   string `json:"agent_id"`
	RoleLabel string `json:"role_label"`
	OrderIdx  int    `json:"order_idx"`
	Mode      string `json:"mode"` // "seq" | "par" (Phase 1: seq doang)
	Optional  bool   `json:"optional"`
}

type TaskRun struct {
	ID          int64      `json:"id"`
	CategoryID  string     `json:"category_id"`
	InputText   string     `json:"input_text"`
	Status      string     `json:"status"` // running|done|error
	RequestedBy string     `json:"requested_by"`
	Summary     string     `json:"summary"`
	StartedAt   string     `json:"started_at"`
	FinishedAt  string     `json:"finished_at"`
	Steps       []TaskStep `json:"steps,omitempty"`
}

type TaskStep struct {
	ID        int64  `json:"id"`
	RunID     int64  `json:"run_id"`
	AgentID   string `json:"agent_id"`
	RoleLabel string `json:"role_label"`
	OrderIdx  int    `json:"order_idx"`
	Status    string `json:"status"` // pending|running|done|error
	OutputRef string `json:"output_ref"`
	Err       string `json:"err"`
	MS        int64  `json:"ms"`
}

// ── Schema (idempotent; dipanggil EnsureTaskSchema, ga sentuh floworkdb.go) ──

// EnsureTaskSchema bikin tabel task kalau belum ada. Aman dipanggil berkali2.
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
	return nil
}

// ── Category CRUD ────────────────────────────────────────────────────────────

// UpsertCategory insert/update kategori (tanpa crew — crew via SetCrew).
func (s *Store) UpsertCategory(c TaskCategory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := 0
	if c.Enabled {
		en = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO task_categories(id,name,icon,trigger_hint,synthesizer,enabled)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, icon=excluded.icon,
		   trigger_hint=excluded.trigger_hint, synthesizer=excluded.synthesizer, enabled=excluded.enabled`,
		c.ID, c.Name, c.Icon, c.TriggerHint, c.Synthesizer, en)
	return err
}

func (s *Store) DeleteCategory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM task_agents WHERE category_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM task_categories WHERE id=?`, id)
	return err
}

// SetCrew ganti seluruh crew 1 kategori (replace).
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

// GetCategory ambil 1 kategori + crew-nya (ordered).
func (s *Store) GetCategory(id string) (*TaskCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := TaskCategory{}
	var en int
	err := s.db.QueryRow(
		`SELECT id,name,icon,trigger_hint,synthesizer,enabled FROM task_categories WHERE id=?`, id,
	).Scan(&c.ID, &c.Name, &c.Icon, &c.TriggerHint, &c.Synthesizer, &en)
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

// ListCategories semua kategori (tanpa crew, ringan buat list view).
func (s *Store) ListCategories() ([]TaskCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT id,name,icon,trigger_hint,synthesizer,enabled FROM task_categories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskCategory
	for rows.Next() {
		var c TaskCategory
		var en int
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.TriggerHint, &c.Synthesizer, &en); err != nil {
			return nil, err
		}
		c.Enabled = en == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── Run persistence (timeline) ───────────────────────────────────────────────

func (s *Store) CreateRun(categoryID, input, requestedBy string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO task_runs(category_id,input_text,status,requested_by) VALUES(?,?,'running',?)`,
		categoryID, input, requestedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// StartStep catat step mulai (status running) → return step id buat FinishStep.
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

// ListRuns N run terakhir 1 kategori (ringkas, tanpa steps).
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

// GetRun 1 run + steps (buat timeline live).
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

// MarkRunningInterrupted — boot hygiene: run yang status 'running' dari proses
// SEBELUMNYA (mati/restart) ga akan pernah kelar (goroutine-nya ilang). Tandai
// 'interrupted' biar ga zombie "running" selamanya di timeline.
func (s *Store) MarkRunningInterrupted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE task_runs SET status='interrupted', finished_at=datetime('now')
		 WHERE status='running'`)
	return err
}

// ── Seed (crew SAHAM Fase 4 → DB, kalau kosong) ──────────────────────────────

// SeedSahamIfEmpty isi kategori SAHAM (mirror taskflow.go Phase 1) kalau belum
// ada kategori sama sekali. Idempotent.
func (s *Store) SeedSahamIfEmpty() error {
	if err := s.EnsureTaskSchema(); err != nil {
		return err
	}
	cats, err := s.ListCategories()
	if err != nil {
		return err
	}
	if len(cats) > 0 {
		return nil // udah ada — jangan timpa editan owner
	}
	if err := s.UpsertCategory(TaskCategory{
		ID: "saham", Name: "Analisa Saham", Icon: "📈",
		TriggerHint: "analisa saham <kode>", Synthesizer: "saham-sinteser", Enabled: true,
	}); err != nil {
		return err
	}
	return s.SetCrew("saham", []TaskAgent{
		{AgentID: "saham-fundamental", RoleLabel: "analis fundamental (bisnis, valuasi, prospek)", Mode: "seq"},
		{AgentID: "saham-keuangan", RoleLabel: "analis laporan keuangan (revenue, laba, utang, arus kas)", Mode: "seq"},
		{AgentID: "saham-teknikal", RoleLabel: "analis teknikal (tren harga, support/resistance, momentum)", Mode: "seq"},
	})
}
