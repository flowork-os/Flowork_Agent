// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

const schemaSQL = `
CREATE TABLE IF NOT EXISTS _meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	data TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS providerConnections (
	id TEXT PRIMARY KEY,
	provider TEXT NOT NULL,
	authType TEXT NOT NULL,
	name TEXT,
	email TEXT,
	priority INTEGER DEFAULT 0,
	isActive INTEGER DEFAULT 1,
	data TEXT NOT NULL DEFAULT '{}',
	createdAt TEXT NOT NULL,
	updatedAt TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pc_provider ON providerConnections(provider);
CREATE INDEX IF NOT EXISTS idx_pc_provider_active ON providerConnections(provider, isActive);
CREATE INDEX IF NOT EXISTS idx_pc_priority ON providerConnections(provider, priority);

CREATE TABLE IF NOT EXISTS providerNodes (
	id TEXT PRIMARY KEY,
	type TEXT,
	name TEXT,
	data TEXT NOT NULL DEFAULT '{}',
	createdAt TEXT NOT NULL,
	updatedAt TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pn_type ON providerNodes(type);

CREATE TABLE IF NOT EXISTS apiKeys (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	keyHash TEXT NOT NULL UNIQUE,
	keyPrefix TEXT NOT NULL,
	allowedProviders TEXT DEFAULT '*',
	dailyCapUsd REAL DEFAULT 0,
	monthlyCapUsd REAL DEFAULT 0,
	isActive INTEGER DEFAULT 1,
	createdAt TEXT NOT NULL,
	lastUsedAt TEXT
);

CREATE TABLE IF NOT EXISTS usageDaily (
	day TEXT NOT NULL,
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	apiKeyId TEXT,
	requestCount INTEGER DEFAULT 0,
	promptTokens INTEGER DEFAULT 0,
	completionTokens INTEGER DEFAULT 0,
	costUsd REAL DEFAULT 0,
	PRIMARY KEY (day, provider, model, apiKeyId)
);
CREATE INDEX IF NOT EXISTS idx_ud_day ON usageDaily(day);

CREATE TABLE IF NOT EXISTS usageHistory (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TEXT NOT NULL,
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	apiKeyId TEXT,
	promptTokens INTEGER DEFAULT 0,
	completionTokens INTEGER DEFAULT 0,
	costUsd REAL DEFAULT 0,
	latencyMs INTEGER DEFAULT 0,
	status TEXT DEFAULT 'ok'
);
CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(ts);
CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider);

CREATE TABLE IF NOT EXISTS requestDetails (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TEXT NOT NULL,
	apiKeyId TEXT,
	providerId TEXT,
	model TEXT,
	clientIp TEXT,
	clientUA TEXT,
	requestBody TEXT,
	responseBody TEXT,
	statusCode INTEGER,
	error TEXT,
	durationMs INTEGER
);
CREATE INDEX IF NOT EXISTS idx_rd_ts ON requestDetails(ts);

CREATE TABLE IF NOT EXISTS brainContributions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TEXT NOT NULL,
	agent TEXT,
	model TEXT,
	mode TEXT,
	query TEXT NOT NULL,
	sources TEXT,
	answer TEXT,
	ingested INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bc_ingested ON brainContributions(ingested);

CREATE TABLE IF NOT EXISTS combos (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	models TEXT NOT NULL DEFAULT '[]',
	strategy TEXT DEFAULT 'priority',
	createdAt TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS proxyPools (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	proxies TEXT NOT NULL DEFAULT '[]',
	rotation TEXT DEFAULT 'round-robin',
	isActive INTEGER DEFAULT 1,
	createdAt TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS kv (
	k TEXT PRIMARY KEY,
	v TEXT NOT NULL,
	updatedAt TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tags (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT DEFAULT '#8b5cf6',
	kind TEXT DEFAULT 'generic',
	createdAt TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pricing (
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	inputUsdPer1M REAL DEFAULT 0,
	outputUsdPer1M REAL DEFAULT 0,
	cacheReadUsdPer1M REAL DEFAULT 0,
	cacheWriteUsdPer1M REAL DEFAULT 0,
	currency TEXT DEFAULT 'USD',
	source TEXT,
	updatedAt TEXT NOT NULL,
	PRIMARY KEY (provider, model)
);
CREATE INDEX IF NOT EXISTS idx_pricing_provider ON pricing(provider);

CREATE TABLE IF NOT EXISTS modelAlias (
	alias TEXT PRIMARY KEY,
	providerId TEXT NOT NULL,
	model TEXT NOT NULL,
	createdAt TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_modelAlias_provider ON modelAlias(providerId);

CREATE TABLE IF NOT EXISTS modelAvailability (
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	status TEXT DEFAULT 'unknown',
	latencyMs INTEGER DEFAULT 0,
	checkedAt TEXT NOT NULL,
	errorMessage TEXT,
	PRIMARY KEY (provider, model)
);
CREATE INDEX IF NOT EXISTS idx_modelAvail_status ON modelAvailability(status);

CREATE TABLE IF NOT EXISTS authSessions (
	id TEXT PRIMARY KEY,
	token TEXT NOT NULL UNIQUE,
	userId TEXT,
	createdAt TEXT NOT NULL,
	expiresAt TEXT NOT NULL,
	lastSeenAt TEXT,
	ip TEXT,
	userAgent TEXT
);
CREATE INDEX IF NOT EXISTS idx_authSessions_expires ON authSessions(expiresAt);

CREATE TABLE IF NOT EXISTS translatorDrafts (
	id TEXT PRIMARY KEY,
	name TEXT,
	sourceFormat TEXT NOT NULL,
	targetFormat TEXT NOT NULL,
	input TEXT,
	output TEXT,
	createdAt TEXT NOT NULL,
	updatedAt TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_translator_updated ON translatorDrafts(updatedAt);

CREATE TABLE IF NOT EXISTS modelsCustom (
	id TEXT PRIMARY KEY,
	providerId TEXT,
	model TEXT NOT NULL,
	displayName TEXT,
	contextWindow INTEGER DEFAULT 0,
	maxOutputTokens INTEGER DEFAULT 0,
	supportsTools INTEGER DEFAULT 0,
	supportsVision INTEGER DEFAULT 0,
	supportsStreaming INTEGER DEFAULT 1,
	createdAt TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS modelsDisabled (
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	disabledAt TEXT NOT NULL,
	reason TEXT,
	PRIMARY KEY (provider, model)
);
`

var (
	dbOnce sync.Once
	db     *sql.DB
	dbErr  error
)

func dataDir() string {
	if d := os.Getenv("FLOW_ROUTER_DATA"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".flow_router")
}

func DBPath() string {
	return filepath.Join(dataDir(), "db", "data.sqlite")
}

func Open() (*sql.DB, error) {
	dbOnce.Do(func() {
		p := DBPath()
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			dbErr = fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
			return
		}
		conn, err := sql.Open("sqlite", p+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
		if err != nil {
			dbErr = fmt.Errorf("open %s: %w", p, err)
			return
		}
		if err := conn.Ping(); err != nil {
			dbErr = fmt.Errorf("ping: %w", err)
			return
		}
		if _, err := conn.Exec(schemaSQL); err != nil {
			dbErr = fmt.Errorf("schema apply: %w", err)
			return
		}

		_, _ = conn.Exec(`INSERT OR REPLACE INTO _meta (key, value) VALUES ('schemaVersion', ?), ('appVersion', '0.1.0')`, fmt.Sprintf("%d", schemaVersion))
		db = conn

		if err := applyMigrations(conn); err != nil {
			dbErr = fmt.Errorf("migrate: %w", err)
			_ = conn.Close()
			db = nil
			return
		}
	})
	if dbErr != nil {
		return nil, dbErr
	}
	return db, nil
}

func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
