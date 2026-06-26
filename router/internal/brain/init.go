// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

const brainSchemaSQL = `
CREATE TABLE IF NOT EXISTS agents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT '',
	system_prompt TEXT DEFAULT '',
	model TEXT DEFAULT '',
	status TEXT DEFAULT 'idle',
	daemon_cmd TEXT DEFAULT '',
	env_prefix TEXT DEFAULT '',
	workspace_path TEXT DEFAULT '',
	model_name TEXT DEFAULT '',
	capability_tier INTEGER DEFAULT 0,
	tier_unlocked_at TEXT DEFAULT '',
	prompt_template TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_agents_name ON agents(name);

CREATE TABLE IF NOT EXISTS constitution (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source_file TEXT NOT NULL,
	section TEXT NOT NULL,
	content TEXT NOT NULL,
	amplitude REAL DEFAULT 999999.0,
	pending_quorum_review INTEGER DEFAULT 0,
	deleted_at TIMESTAMP,
	deleted_by TEXT,
	sacred_lens INTEGER DEFAULT 0,
	is_catalyst INTEGER DEFAULT 0,
	context_origin TEXT,
	cross_refs TEXT DEFAULT '[]',
	cross_refs_typed TEXT DEFAULT '{}',
	signature TEXT,
	signer TEXT,
	origin_node TEXT DEFAULT 'local',
	synced_from TEXT,
	synced_at TEXT,
	UNIQUE(source_file, section)
);
CREATE INDEX IF NOT EXISTS idx_constitution_source ON constitution(source_file);
CREATE INDEX IF NOT EXISTS idx_constitution_deleted_at ON constitution(deleted_at);

CREATE TABLE IF NOT EXISTS drawers (
	id TEXT PRIMARY KEY,
	content TEXT NOT NULL,
	wing TEXT NOT NULL DEFAULT '',
	room TEXT NOT NULL DEFAULT '',
	source_file TEXT DEFAULT '',
	source_type TEXT DEFAULT 'manual',
	chunk_index INTEGER DEFAULT 0,
	importance REAL DEFAULT 3.0,
	normalize_version INTEGER DEFAULT 1,
	filed_at TEXT DEFAULT CURRENT_TIMESTAMP,
	content_hash TEXT NOT NULL DEFAULT '',
	deleted_at TIMESTAMP,
	deleted_by TEXT,
	quarantined INTEGER DEFAULT 0,
	reason_quarantine TEXT DEFAULT '',
	mem_type TEXT NOT NULL DEFAULT 'project'
);
CREATE INDEX IF NOT EXISTS idx_drawers_wing ON drawers(wing);
CREATE INDEX IF NOT EXISTS idx_drawers_room ON drawers(room);
CREATE INDEX IF NOT EXISTS idx_drawers_source ON drawers(source_file);
CREATE INDEX IF NOT EXISTS idx_drawers_hash ON drawers(content_hash);
CREATE INDEX IF NOT EXISTS idx_drawers_wing_room ON drawers(wing, room);
CREATE INDEX IF NOT EXISTS idx_drawers_importance ON drawers(importance DESC);
CREATE INDEX IF NOT EXISTS idx_drawers_source_type ON drawers(source_type);
CREATE INDEX IF NOT EXISTS idx_drawers_deleted_at ON drawers(deleted_at);
CREATE INDEX IF NOT EXISTS idx_drawers_quarantined ON drawers(quarantined);
CREATE INDEX IF NOT EXISTS idx_drawers_mem_type ON drawers(mem_type);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
	drawer_id UNINDEXED,
	content,
	wing,
	room,
	source_file,
	tokenize='porter unicode61'
);

CREATE TABLE IF NOT EXISTS memories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	agent_id INTEGER NOT NULL,
	category TEXT NOT NULL,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	source_path TEXT DEFAULT '',
	amplitude REAL DEFAULT 1.0,
	deleted_at TIMESTAMP,
	deleted_by TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(agent_id, source_path)
);
CREATE INDEX IF NOT EXISTS idx_memories_agent ON memories(agent_id);
CREATE INDEX IF NOT EXISTS idx_memories_cat ON memories(category);
CREATE INDEX IF NOT EXISTS idx_memories_deleted_at ON memories(deleted_at);

CREATE TABLE IF NOT EXISTS skills (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	agent_id INTEGER NOT NULL,
	skill_name TEXT NOT NULL,
	content TEXT NOT NULL,
	version INTEGER DEFAULT 1,
	amplitude REAL DEFAULT 1.0,
	active INTEGER DEFAULT 1,
	deleted_at TIMESTAMP,
	deleted_by TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(agent_id, skill_name)
);
CREATE INDEX IF NOT EXISTS idx_skills_agent ON skills(agent_id);
CREATE INDEX IF NOT EXISTS idx_skills_deleted_at ON skills(deleted_at);

CREATE TABLE IF NOT EXISTS tool_patterns (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	trigger_pattern TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	arguments_template TEXT DEFAULT '{}',
	success_count INTEGER DEFAULT 0,
	fail_count INTEGER DEFAULT 0,
	amplitude REAL DEFAULT 0.5,
	deleted_at TIMESTAMP,
	deleted_by TEXT,
	UNIQUE(trigger_pattern, tool_name)
);
CREATE INDEX IF NOT EXISTS idx_tool_patterns_tool ON tool_patterns(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_patterns_deleted_at ON tool_patterns(deleted_at);

CREATE TABLE IF NOT EXISTS prompt_templates (
	name TEXT PRIMARY KEY,
	content TEXT NOT NULL DEFAULT '',
	source_path TEXT DEFAULT '',
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Section 7 — Mistakes journal (global tier).
-- Receive promotion dari agent (per agent roadmap section 2 + 7).
-- UNIQUE(category, title) → upsert hit_count semantik.
CREATE TABLE IF NOT EXISTS mistakes_journal (
	id                      INTEGER PRIMARY KEY AUTOINCREMENT,
	category                TEXT NOT NULL,
	title                   TEXT NOT NULL,
	content                 TEXT NOT NULL,
	source_agent_id         TEXT NOT NULL,
	hit_count               INTEGER DEFAULT 1,
	tier                    TEXT DEFAULT 'global',
	reviewed_at             TEXT,
	promoted_to_antibody_id TEXT,
	created_at              TEXT DEFAULT CURRENT_TIMESTAMP,
	updated_at              TEXT DEFAULT CURRENT_TIMESTAMP,
	deleted_at              TIMESTAMP,
	UNIQUE(category, title)
);
CREATE INDEX IF NOT EXISTS idx_mistakes_journal_tier      ON mistakes_journal(tier);
CREATE INDEX IF NOT EXISTS idx_mistakes_journal_source    ON mistakes_journal(source_agent_id);
CREATE INDEX IF NOT EXISTS idx_mistakes_journal_deleted   ON mistakes_journal(deleted_at);
CREATE INDEX IF NOT EXISTS idx_mistakes_journal_updated   ON mistakes_journal(updated_at DESC);

-- Section 10 — Recorder (existing schema in brain reused, NO new table).
-- See internal/recorder/recorder.go for column mapping.
`

func EnsureSchema() (bool, error) {
	p := DBPath()
	if p == "" {
		return false, fmt.Errorf("no brain DB path configured")
	}
	fresh := false
	if _, err := os.Stat(p); err != nil {
		fresh = true
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
		}
	}

	dsn := "file:" + p + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(0)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", p, err)
	}
	defer db.Close()
	if _, err := db.Exec(brainSchemaSQL); err != nil {
		return false, fmt.Errorf("apply schema: %w", err)
	}

	if fresh {
		invalidateHandles()
	}
	return fresh, nil
}

func invalidateHandles() {
	handleMu.Lock()
	if handle != nil {
		_ = handle.Close()
		handle = nil
		handleP = ""
	}
	handleMu.Unlock()
	rwMu.Lock()
	if rwHandle != nil {
		_ = rwHandle.Close()
		rwHandle = nil
		rwPath = ""
	}
	rwMu.Unlock()
}
