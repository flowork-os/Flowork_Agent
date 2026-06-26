// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

func init() {
	RegisterMigration(Migration{
		ID:   102,
		Name: "section24_27_llm_pricing_policy",
		SQL: `
-- Section 24: provider chain + call log
CREATE TABLE IF NOT EXISTS provider_chain_configs (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  chain_name     TEXT NOT NULL UNIQUE,
  providers_json TEXT NOT NULL DEFAULT '[]',
  created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS provider_call_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  caller        TEXT NOT NULL DEFAULT '',
  provider      TEXT NOT NULL,
  model         TEXT NOT NULL,
  input_tokens  INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cost_usd      REAL NOT NULL DEFAULT 0,
  latency_ms    INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL DEFAULT 'success',
  occurred_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_provider_call_log_time ON provider_call_log(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_provider_call_log_caller ON provider_call_log(caller);

-- Section 25: localai model registry
CREATE TABLE IF NOT EXISTS localai_models (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  model_name   TEXT NOT NULL UNIQUE,
  gguf_path    TEXT NOT NULL DEFAULT '',
  size_bytes   INTEGER NOT NULL DEFAULT 0,
  checksum     TEXT NOT NULL DEFAULT '',
  manifest_json TEXT NOT NULL DEFAULT '{}',
  signature    TEXT NOT NULL DEFAULT '',
  loaded       INTEGER NOT NULL DEFAULT 0,
  added_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Section 26: pricing extension (existing 'pricing' table sudah ada,
-- tambah 'pricing_rules' yang allow tiered + per-warga override)
CREATE TABLE IF NOT EXISTS pricing_rules (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_name    TEXT NOT NULL,
  provider     TEXT NOT NULL,
  model        TEXT NOT NULL,
  tier         TEXT NOT NULL DEFAULT 'default',
  input_per_1m_usd  REAL NOT NULL DEFAULT 0,
  output_per_1m_usd REAL NOT NULL DEFAULT 0,
  enabled      INTEGER NOT NULL DEFAULT 1,
  notes        TEXT NOT NULL DEFAULT '',
  UNIQUE(provider, model, tier)
);

-- Section 27: policy budget (per-warga + global)
CREATE TABLE IF NOT EXISTS policy_budgets (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  scope          TEXT NOT NULL,
  scope_key      TEXT NOT NULL DEFAULT '',
  metric_key     TEXT NOT NULL,
  budget_value   REAL NOT NULL,
  reset_period   TEXT NOT NULL DEFAULT 'daily',
  warning_pct    REAL NOT NULL DEFAULT 0.8,
  enabled        INTEGER NOT NULL DEFAULT 1,
  created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(scope, scope_key, metric_key)
);

CREATE TABLE IF NOT EXISTS policy_violations (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  budget_id    INTEGER NOT NULL,
  fired_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  actual_value REAL NOT NULL,
  action_taken TEXT NOT NULL DEFAULT 'warn'
);
CREATE INDEX IF NOT EXISTS idx_policy_violations_budget ON policy_violations(budget_id);
`,
	})
}
