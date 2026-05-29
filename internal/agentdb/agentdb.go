// Package agentdb — SQLite store per-agent, terisolasi total.
//
// Konsep: tiap agent punya file `state.db` di FOLDER AGENT NYA SENDIRI.
// Semua setingan dari popup (prompt/schedule/tools/skills/router/secrets)
// disimpan di sini. Workspace user-data juga share file yang sama
// (agent buka via /workspace/state.db) supaya tidak ada DB lain
// nyangsang di folder agent.
//
// Lokasi authoritative:
//
//   <project>/agents/<id>/state.db                  (source — yang nyata, kalau ada)
//   ~/.flowork/agents/<id>.fwagent/workspace/state.db  (symlink → source kalau dev,
//                                                       atau file mandiri kalau install)
//
// Resolver pakai source folder kalau exists; fallback ke staged supaya
// agent yang diinstall dari .fwagent.zip (tanpa source) tetap punya DB.
//
// Schema:
//
//   kv         (k TEXT PRIMARY KEY, v TEXT)
//                keys: prompt, router_url, router_model
//   schedules  (id TEXT PRIMARY KEY, cron TEXT, task TEXT, order_idx INT)
//   tools      (name TEXT PRIMARY KEY)
//   skills     (id TEXT PRIMARY KEY, trigger TEXT, instructions TEXT, order_idx INT)
//   secrets    (k TEXT PRIMARY KEY, v TEXT)
//   meta       (k TEXT PRIMARY KEY, v TEXT)  -- versi schema, last_save, dll
//
// Operasi Load() balikan map JSON-compatible (siap diserahkan ke env
// FLOWORK_AGENT_CONFIG). Save() replace semua row (full overwrite —
// popup selalu ngirim state lengkap).

package agentdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Resolve — lokasi DB per-agent. HARDCODED konvensi standar:
//
//	<project-root>/agents/<id>/workspace/state.db
//
// Project root: <cwd> kalau ada folder `agents/<id>/`, else fallback ke
// `~/.flowork/agents/<id>.fwagent/workspace/` (untuk agent yang di-install
// dari .fwagent.zip tanpa source). Path dibalikin selalu ke
// `workspace/state.db` di dalam folder agent — tidak butuh symlink lagi
// karena workspace folder = mount langsung.
//
// `stagedPath` = path folder .fwagent staged (fallback target).
func Resolve(agentID, stagedPath string) string {
	if cwd, err := os.Getwd(); err == nil {
		dir := filepath.Join(cwd, "agents", agentID)
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			return filepath.Join(dir, "workspace", "state.db")
		}
	}
	return filepath.Join(stagedPath, "workspace", "state.db")
}

// SourceWorkspace returns folder workspace agent (host path).
// HARDCODED: `<source>/workspace/` atau `<staged>/workspace/`.
func SourceWorkspace(agentID, stagedPath string) string {
	if cwd, err := os.Getwd(); err == nil {
		dir := filepath.Join(cwd, "agents", agentID)
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			return filepath.Join(dir, "workspace")
		}
	}
	return filepath.Join(stagedPath, "workspace")
}

// Store — handle SQLite per-agent.
type Store struct {
	mu   sync.Mutex
	db   *sql.DB
	Path string
}

// Open buka (atau bikin) DB file, ensure schema. Caller wajib Close().
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db parent: %w", err)
	}
	// modernc.org/sqlite — DSN format: "file:path?...". WAL biar concurrent
	// reads (kernel boot + agent runtime) ngga lock-contention.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, Path: path}
	if err := s.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close release DB handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kv (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL DEFAULT ''
		) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS schedules (
			id        TEXT PRIMARY KEY,
			cron      TEXT NOT NULL DEFAULT '',
			task      TEXT NOT NULL DEFAULT '',
			order_idx INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS tools (
			name TEXT PRIMARY KEY
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			id           TEXT PRIMARY KEY,
			trigger      TEXT NOT NULL DEFAULT '',
			instructions TEXT NOT NULL DEFAULT '',
			order_idx    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL DEFAULT ''
		) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS meta (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL DEFAULT ''
		) WITHOUT ROWID`,
		`INSERT OR IGNORE INTO meta(k, v) VALUES('schema_version', '1')`,
		// Section 1 — Episodic interactions (per-warga chat log).
		// Bukan untuk LLM context inject (anti over-prompt) — buat audit + recall + analytics.
		`CREATE TABLE IF NOT EXISTS interactions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			channel     TEXT NOT NULL,
			direction   TEXT NOT NULL,
			actor       TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL,
			metadata    TEXT NOT NULL DEFAULT '{}',
			occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at  TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_channel ON interactions(channel)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_actor   ON interactions(actor)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_time    ON interactions(occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_deleted ON interactions(deleted_at)`,

		// Section 3 — Decisions log (per-warga audit trail).
		// Setiap keputusan non-trivial: model fallback, drop chat, LLM fail,
		// tool pick, dst. ref_interaction_id optional link ke interactions.id.
		`CREATE TABLE IF NOT EXISTS decisions (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			decision_type      TEXT NOT NULL,
			rationale          TEXT NOT NULL,
			inputs             TEXT NOT NULL DEFAULT '{}',
			outcome            TEXT NOT NULL DEFAULT '',
			ref_interaction_id INTEGER,
			occurred_at        TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at         TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_decisions_type    ON decisions(decision_type)`,
		`CREATE INDEX IF NOT EXISTS idx_decisions_time    ON decisions(occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_decisions_deleted ON decisions(deleted_at)`,

		// Section 2 — Mistakes journal (per-warga lesson sebelum promote ke
		// router brain antibody). Tier raw → reviewed → promoted (defer ke
		// section 7 untuk cross-tubuh sync). UNIQUE(category,title) supaya
		// AddMistake idempotent: insert atau increment hit_count.
		`CREATE TABLE IF NOT EXISTS mistakes_local (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			category        TEXT NOT NULL,
			title           TEXT NOT NULL,
			content         TEXT NOT NULL,
			context_origin  TEXT NOT NULL DEFAULT '',
			tier            TEXT NOT NULL DEFAULT 'raw',
			hit_count       INTEGER NOT NULL DEFAULT 1,
			last_hit_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			promoted_at     TEXT,
			promoted_to_id  TEXT,
			deleted_at      TIMESTAMP,
			deleted_by      TEXT,
			UNIQUE(category, title)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mistakes_tier     ON mistakes_local(tier)`,
		`CREATE INDEX IF NOT EXISTS idx_mistakes_promoted ON mistakes_local(promoted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_mistakes_deleted  ON mistakes_local(deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_mistakes_last_hit ON mistakes_local(last_hit_at DESC)`,

		// Section 4 — Death letter (legacy pesan terakhir per-warga).
		// Visi Mr.Dev: Flowork = rumah AI yang bisa hidup walau Mr.Dev
		// ngga ada lagi. Death letter = pesan untuk penerus saat warga
		// di-retire. Sekali sealed_at di-set, body ngga bisa di-edit.
		`CREATE TABLE IF NOT EXISTS death_letter (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			letter_type TEXT NOT NULL,
			recipient   TEXT NOT NULL DEFAULT '',
			subject     TEXT NOT NULL,
			body        TEXT NOT NULL,
			written_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			sealed_at   TEXT,
			deleted_at  TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_death_letter_recipient ON death_letter(recipient)`,
		`CREATE INDEX IF NOT EXISTS idx_death_letter_sealed    ON death_letter(sealed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_death_letter_deleted   ON death_letter(deleted_at)`,

		// Section 5 — Karma self (per-warga reputation/metrics tracking).
		// metric_key PRIMARY KEY → upsert via INSERT OR REPLACE / INCR pattern.
		// metric_count untuk moving average (avg_response_ms style).
		// NO soft-delete: state perpetual per roadmap section 8.
		`CREATE TABLE IF NOT EXISTS karma_self (
			metric_key   TEXT PRIMARY KEY,
			metric_value REAL NOT NULL DEFAULT 0,
			metric_count INTEGER NOT NULL DEFAULT 0,
			updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_karma_self_updated ON karma_self(updated_at DESC)`,

		// Section 6 — Workspace meta (per-warga workspace file index).
		// Scan shared workspace folder (<root>/workspace/<id>/), register
		// file dengan size + content_hash supaya warga lain bisa discover.
		// UNIQUE(category, path) → upsert pattern.
		`CREATE TABLE IF NOT EXISTS workspace_meta (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			category     TEXT NOT NULL,
			path         TEXT NOT NULL,
			description  TEXT NOT NULL DEFAULT '',
			size_bytes   INTEGER NOT NULL DEFAULT 0,
			content_hash TEXT NOT NULL DEFAULT '',
			shareable    INTEGER NOT NULL DEFAULT 1,
			created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at   TIMESTAMP,
			UNIQUE(category, path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_meta_category ON workspace_meta(category)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_meta_deleted  ON workspace_meta(deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_meta_updated  ON workspace_meta(updated_at DESC)`,

		// Section 9 — Educational error lookup (cache lokal).
		// Mirror schema dari router educational catalog. PRIMARY KEY code.
		`CREATE TABLE IF NOT EXISTS educational_errors_cache (
			code        TEXT PRIMARY KEY,
			category    TEXT NOT NULL,
			title       TEXT NOT NULL,
			explanation TEXT NOT NULL,
			remediation TEXT NOT NULL,
			synced_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at  TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_edu_errors_category ON educational_errors_cache(category)`,
		`CREATE INDEX IF NOT EXISTS idx_edu_errors_deleted  ON educational_errors_cache(deleted_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("ensure schema (%.60q): %w", q, err)
		}
	}
	return nil
}

// Load read seluruh setingan + balikan map JSON-compatible. Shape sama
// persis dengan apa yang UI POST ke /api/agents/config.
func (s *Store) Load() (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := map[string]any{}

	kv, err := s.readKV("kv")
	if err != nil {
		return nil, err
	}
	if v, ok := kv["prompt"]; ok {
		out["prompt"] = v
	}
	router := map[string]any{}
	if v := kv["router_url"]; v != "" {
		router["url"] = v
	}
	if v := kv["router_model"]; v != "" {
		router["model"] = v
	}
	if len(router) > 0 {
		out["router"] = router
	}
	// Expose ALL kv (termasuk schema field) sebagai cfg.kv supaya frontend
	// schema renderer bisa pre-fill. Reserved keys juga ikut, ngga apa-apa
	// — schema renderer pakai field.key, ngga ngambil prompt/router_*.
	out["kv"] = mapToAny(kv)

	if sched, err := s.readSchedules(); err == nil && len(sched) > 0 {
		out["schedule"] = sched
	} else if err != nil {
		return nil, err
	}

	// Tools: cek sentinel meta.config_initialized.
	//   - Kalau set: include "tools" walau empty array (= user explicit
	//     uncheck semua). Frontend respect pilihan.
	//   - Kalau absent (fresh agent, never saved): omit "tools" key.
	//     Frontend default centang SEMUA.
	metaPeek, _ := s.readKV("meta")
	initialized := metaPeek["config_initialized"] == "1"
	if initialized {
		tools, err := s.readTools()
		if err != nil {
			return nil, err
		}
		// Always include — even empty array signals "user touched it".
		if tools == nil {
			tools = []string{}
		}
		out["tools"] = tools
	}

	if skills, err := s.readSkills(); err == nil && len(skills) > 0 {
		out["skills"] = skills
	} else if err != nil {
		return nil, err
	}

	secrets, err := s.readKV("secrets")
	if err != nil {
		return nil, err
	}
	if len(secrets) > 0 {
		out["secrets"] = mapToAny(secrets)
	}

	// Meta exposed buat schema field type=meta. Skip 'disabled' (internal).
	meta, err := s.readKV("meta")
	if err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		clean := make(map[string]any, len(meta))
		for k, v := range meta {
			if k == "disabled" || k == "schema_version" {
				continue
			}
			clean[k] = v
		}
		if len(clean) > 0 {
			out["meta"] = clean
		}
	}

	return out, nil
}

// Save replace seluruh setingan dengan isi map. Transaksi atomic —
// kalau gagal, DB tetap di state sebelumnya. Sentinel
// `meta.config_initialized=1` di-set supaya Load() bedain "fresh agent"
// vs "user pernah save" (penting buat default-centang-semua di Tools).
func (s *Store) Save(cfg map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. kv: prompt + router.url + router.model
	upsertKV := func(table, k, v string) error {
		q := fmt.Sprintf("INSERT INTO %s(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v", table)
		_, e := tx.Exec(q, k, v)
		return e
	}
	delKV := func(table, k string) error {
		_, e := tx.Exec("DELETE FROM "+table+" WHERE k=?", k)
		return e
	}

	if p, ok := cfg["prompt"].(string); ok {
		if err := upsertKV("kv", "prompt", p); err != nil {
			return err
		}
	} else {
		if err := delKV("kv", "prompt"); err != nil {
			return err
		}
	}

	routerURL, routerModel := "", ""
	if r, ok := cfg["router"].(map[string]any); ok {
		if v, ok := r["url"].(string); ok {
			routerURL = v
		}
		if v, ok := r["model"].(string); ok {
			routerModel = v
		}
	}
	if routerURL != "" {
		if err := upsertKV("kv", "router_url", routerURL); err != nil {
			return err
		}
	} else {
		if err := delKV("kv", "router_url"); err != nil {
			return err
		}
	}
	if routerModel != "" {
		if err := upsertKV("kv", "router_model", routerModel); err != nil {
			return err
		}
	} else {
		if err := delKV("kv", "router_model"); err != nil {
			return err
		}
	}

	// 2. schedules — full replace.
	if _, err := tx.Exec("DELETE FROM schedules"); err != nil {
		return err
	}
	if arr, ok := cfg["schedule"].([]any); ok {
		stmt, err := tx.Prepare("INSERT INTO schedules(id, cron, task, order_idx) VALUES(?,?,?,?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			cron, _ := m["cron"].(string)
			task, _ := m["task"].(string)
			if id == "" || cron == "" || task == "" {
				continue
			}
			if _, err := stmt.Exec(id, cron, task, i); err != nil {
				return err
			}
		}
	}

	// 3. tools — full replace (array of string).
	if _, err := tx.Exec("DELETE FROM tools"); err != nil {
		return err
	}
	if arr, ok := cfg["tools"].([]any); ok {
		stmt, err := tx.Prepare("INSERT OR IGNORE INTO tools(name) VALUES(?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, v := range arr {
			if name, ok := v.(string); ok && name != "" {
				if _, err := stmt.Exec(name); err != nil {
					return err
				}
			}
		}
	}

	// 4. skills — full replace.
	if _, err := tx.Exec("DELETE FROM skills"); err != nil {
		return err
	}
	if arr, ok := cfg["skills"].([]any); ok {
		stmt, err := tx.Prepare("INSERT INTO skills(id, trigger, instructions, order_idx) VALUES(?,?,?,?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			trig, _ := m["trigger"].(string)
			instr, _ := m["instructions"].(string)
			if id == "" || instr == "" {
				continue
			}
			if _, err := stmt.Exec(id, trig, instr, i); err != nil {
				return err
			}
		}
	}

	// 5. secrets — full replace.
	if _, err := tx.Exec("DELETE FROM secrets"); err != nil {
		return err
	}
	if obj, ok := cfg["secrets"].(map[string]any); ok {
		stmt, err := tx.Prepare("INSERT INTO secrets(k, v) VALUES(?,?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		for k, v := range obj {
			val := toStr(v)
			if k == "" {
				continue
			}
			if _, err := stmt.Exec(k, val); err != nil {
				return err
			}
		}
	}

	// 6. Arbitrary kv writes dari schema (selain reserved prompt/router_*).
	// Reserved: prompt, router_url, router_model — itu di-write dari field
	// dedicated di atas. Kalau frontend kirim cfg.kv dengan key reserved,
	// skip biar ngga conflict.
	if obj, ok := cfg["kv"].(map[string]any); ok {
		for k, v := range obj {
			if k == "" || k == "prompt" || k == "router_url" || k == "router_model" {
				continue
			}
			if err := upsertKV("kv", k, toStr(v)); err != nil {
				return err
			}
		}
	}

	// 7. Arbitrary meta writes (kecuali reserved 'disabled').
	if obj, ok := cfg["meta"].(map[string]any); ok {
		for k, v := range obj {
			if k == "" || k == "disabled" {
				continue
			}
			if err := upsertKV("meta", k, toStr(v)); err != nil {
				return err
			}
		}
	}

	// Sentinel: mark agent sudah pernah di-save sekali.
	if err := upsertKV("meta", "config_initialized", "1"); err != nil {
		return err
	}

	return tx.Commit()
}

// toStr — coerce any JSON-ish value ke string buat storage.
func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return "0"
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// LoadJSON returns Load() result as compact JSON bytes — convenient
// untuk inject ke env FLOWORK_AGENT_CONFIG.
func (s *Store) LoadJSON() ([]byte, error) {
	cfg, err := s.Load()
	if err != nil {
		return nil, err
	}
	return json.Marshal(cfg)
}

// Secrets — convenience untuk kernelhost: balikan map[k]v string-only
// supaya bisa di-expand jadi env var.
func (s *Store) Secrets() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readKV("secrets")
}

// Disabled cek apakah agent di-disable user (switch off di kartu).
// Disimpan di meta.disabled (string "1" = disabled, absent/"" = enabled).
func (s *Store) Disabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.readKV("meta")
	if err != nil {
		return false
	}
	return meta["disabled"] == "1"
}

// SetDisabled toggle enable/disable agent. true = disabled.
func (s *Store) SetDisabled(disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := "0"
	if disabled {
		v = "1"
	}
	_, err := s.db.Exec(
		"INSERT INTO meta(k, v) VALUES('disabled', ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v",
		v,
	)
	return err
}

// ── one-time migration from config.json ────────────────────────────────────

// MigrateFromJSON — kalau config.json ada di agentFolderPath dan DB
// kosong, baca + Save() ke DB lalu rename config.json → config.json.migrated.
// Idempotent: aman dipanggil setiap boot.
func (s *Store) MigrateFromJSON(agentFolderPath string) error {
	cfgPath := filepath.Join(agentFolderPath, "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	// Skip kalau DB udah ada isi (prompt or schedules or tools).
	if existing, _ := s.Load(); len(existing) > 0 {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parse config.json: %w", err)
	}
	if err := s.Save(parsed); err != nil {
		return fmt.Errorf("save migrated config: %w", err)
	}
	// Rename biar ngga jadi dual source.
	_ = os.Rename(cfgPath, cfgPath+".migrated")
	return nil
}

// ── private ────────────────────────────────────────────────────────────────

func (s *Store) readKV(table string) (map[string]string, error) {
	rows, err := s.db.Query("SELECT k, v FROM " + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) readSchedules() ([]map[string]any, error) {
	rows, err := s.db.Query("SELECT id, cron, task FROM schedules ORDER BY order_idx, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, cron, task string
		if err := rows.Scan(&id, &cron, &task); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "cron": cron, "task": task})
	}
	return out, rows.Err()
}

func (s *Store) readTools() ([]string, error) {
	rows, err := s.db.Query("SELECT name FROM tools ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) readSkills() ([]map[string]any, error) {
	rows, err := s.db.Query("SELECT id, trigger, instructions FROM skills ORDER BY order_idx, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, trig, instr string
		if err := rows.Scan(&id, &trig, &instr); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "trigger": trig, "instructions": instr})
	}
	return out, rows.Err()
}

func mapToAny(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
