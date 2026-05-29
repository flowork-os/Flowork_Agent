// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 22 phase 1 wallet alert schema. Lazy CREATE.
//   Phase 2 (multi-channel notify, escalation chain, snooze) → tambah
//   file baru, JANGAN modify ini.
//
// wallet_alert.go — Section 22 phase 1: alert config + fired log.

package agentdb

import (
	"fmt"
	"strings"
	"time"
)

// WalletAlertConfig — alert rule. comparator ∈ {"<", "<=", ">", ">="}.
type WalletAlertConfig struct {
	ID             int64  `json:"id"`
	MetricKey      string `json:"metric_key"`      // mis. "total_usd"
	ThresholdValue float64 `json:"threshold_value"`
	Comparator     string `json:"comparator"`
	NotifyChannel  string `json:"notify_channel"`  // 'telegram' | 'log'
	NotifyTarget   string `json:"notify_target"`   // chat_id atau dst
	Enabled        bool   `json:"enabled"`
	LastFiredAt    string `json:"last_fired_at,omitempty"`
}

// WalletAlertFired — audit row tiap kali alert nyala.
type WalletAlertFired struct {
	ID          int64   `json:"id"`
	ConfigID    int64   `json:"config_id"`
	FiredAt     string  `json:"fired_at"`
	MetricValue float64 `json:"metric_value"`
	Message     string  `json:"message"`
}

func (s *Store) ensureWalletAlertSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS wallet_alerts_config (
		  id              INTEGER PRIMARY KEY AUTOINCREMENT,
		  metric_key      TEXT NOT NULL,
		  threshold_value REAL NOT NULL,
		  comparator      TEXT NOT NULL DEFAULT '<',
		  notify_channel  TEXT NOT NULL,
		  notify_target   TEXT NOT NULL DEFAULT '',
		  enabled         INTEGER NOT NULL DEFAULT 1,
		  last_fired_at   TEXT
		);
		CREATE TABLE IF NOT EXISTS wallet_alerts_fired (
		  id           INTEGER PRIMARY KEY AUTOINCREMENT,
		  config_id    INTEGER NOT NULL,
		  fired_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  metric_value REAL NOT NULL,
		  message      TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_wallet_alerts_fired_config
		  ON wallet_alerts_fired(config_id);
	`)
	return err
}

// AddWalletAlert — INSERT + return ID. Comparator default `<`.
func (s *Store) AddWalletAlert(cfg WalletAlertConfig) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletAlertSchema(); err != nil {
		return 0, err
	}
	cfg.MetricKey = strings.TrimSpace(cfg.MetricKey)
	if cfg.MetricKey == "" {
		return 0, fmt.Errorf("metric_key required")
	}
	if cfg.Comparator == "" {
		cfg.Comparator = "<"
	}
	switch cfg.Comparator {
	case "<", "<=", ">", ">=":
	default:
		return 0, fmt.Errorf("invalid comparator %q", cfg.Comparator)
	}
	if cfg.NotifyChannel == "" {
		cfg.NotifyChannel = "log"
	}
	enabled := 1
	if !cfg.Enabled {
		enabled = 0
	}
	res, err := s.db.Exec(
		`INSERT INTO wallet_alerts_config
		   (metric_key, threshold_value, comparator, notify_channel, notify_target, enabled)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		cfg.MetricKey, cfg.ThresholdValue, cfg.Comparator,
		cfg.NotifyChannel, cfg.NotifyTarget, enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListWalletAlerts — semua row.
func (s *Store) ListWalletAlerts() ([]WalletAlertConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletAlertSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, metric_key, threshold_value, comparator,
		        notify_channel, notify_target, enabled,
		        COALESCE(last_fired_at, '')
		 FROM wallet_alerts_config ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WalletAlertConfig{}
	for rows.Next() {
		var c WalletAlertConfig
		var enabled int
		if serr := rows.Scan(&c.ID, &c.MetricKey, &c.ThresholdValue, &c.Comparator,
			&c.NotifyChannel, &c.NotifyTarget, &enabled, &c.LastFiredAt); serr != nil {
			return nil, serr
		}
		c.Enabled = enabled != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteWalletAlert.
func (s *Store) DeleteWalletAlert(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletAlertSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM wallet_alerts_config WHERE id = ?`, id)
	return err
}

// InsertWalletAlertFired — append audit + update last_fired_at di config.
func (s *Store) InsertWalletAlertFired(configID int64, value float64, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletAlertSchema(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO wallet_alerts_fired (config_id, fired_at, metric_value, message)
		 VALUES (?, ?, ?, ?)`, configID, now, value, message); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE wallet_alerts_config SET last_fired_at = ? WHERE id = ?`,
		now, configID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListWalletAlertsFired — paginated DESC.
func (s *Store) ListWalletAlertsFired(limit int) ([]WalletAlertFired, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletAlertSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, config_id, fired_at, metric_value, message
		 FROM wallet_alerts_fired ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WalletAlertFired{}
	for rows.Next() {
		var f WalletAlertFired
		if serr := rows.Scan(&f.ID, &f.ConfigID, &f.FiredAt, &f.MetricValue, &f.Message); serr != nil {
			return nil, serr
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
