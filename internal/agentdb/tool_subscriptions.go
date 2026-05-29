// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 13 phase 2 tool_subscriptions. Schema lazy create — ngga
//   tambah ke ensureSchema (locked). Idempotent via CREATE TABLE IF NOT
//   EXISTS dipanggil oleh ListSubscriptions/Subscribe/etc. Phase 3
//   (config JSON validation, subscription_at index, multi-source) →
//   tambah file baru, JANGAN modify ini.
//
// tool_subscriptions.go — Section 13 phase 2: per-warga tool subscription.
//
// Semantik:
//   Subscription = UX layer di atas capability gate. Bukan authorization —
//   itu broker.IsApproved. Subscription = "tool ini aktif di listing UI
//   + auto-suggest pool".
//   Sumber: 'manual' (admin/user toggle), 'auto_suggest' (router learner),
//   'group:<name>' (preset bundle).

package agentdb

import (
	"fmt"
	"strings"
	"time"
)

// ToolSubscription mirrors `tool_subscriptions` row.
type ToolSubscription struct {
	ToolName     string `json:"tool_name"`
	SubscribedAt string `json:"subscribed_at"`
	Source       string `json:"source"`
	Config       string `json:"config"` // JSON string
}

// ensureToolSubscriptionsSchema — lazy create, idempotent. Caller (semua
// method tool_subscriptions di sini) call first.
func (s *Store) ensureToolSubscriptionsSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tool_subscriptions (
		  tool_name     TEXT PRIMARY KEY,
		  subscribed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  source        TEXT NOT NULL DEFAULT 'manual',
		  config        TEXT NOT NULL DEFAULT '{}'
		);
		CREATE INDEX IF NOT EXISTS idx_tool_subscriptions_source
		  ON tool_subscriptions(source);
	`)
	if err != nil {
		return fmt.Errorf("ensure tool_subscriptions: %w", err)
	}
	return nil
}

// SubscribeTool — upsert subscription. config JSON default '{}'.
func (s *Store) SubscribeTool(toolName, source, configJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolSubscriptionsSchema(); err != nil {
		return err
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return fmt.Errorf("tool_name required")
	}
	if source == "" {
		source = "manual"
	}
	if configJSON == "" {
		configJSON = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO tool_subscriptions (tool_name, subscribed_at, source, config)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(tool_name) DO UPDATE SET
		   source = excluded.source,
		   config = excluded.config`,
		toolName, now, source, configJSON,
	)
	return err
}

// UnsubscribeTool — delete row. Idempotent (no-row = no-op).
func (s *Store) UnsubscribeTool(toolName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolSubscriptionsSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM tool_subscriptions WHERE tool_name = ?`, toolName)
	return err
}

// IsSubscribed — return true kalau row ada.
func (s *Store) IsSubscribed(toolName string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolSubscriptionsSchema(); err != nil {
		return false, err
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tool_subscriptions WHERE tool_name = ?`, toolName,
	).Scan(&n)
	return n > 0, err
}

// ListSubscriptions — return all rows sorted by tool_name. Cap 500.
func (s *Store) ListSubscriptions() ([]ToolSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolSubscriptionsSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT tool_name, subscribed_at, source, config
		 FROM tool_subscriptions
		 ORDER BY tool_name
		 LIMIT 500`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ToolSubscription{}
	for rows.Next() {
		var ts ToolSubscription
		if serr := rows.Scan(&ts.ToolName, &ts.SubscribedAt, &ts.Source, &ts.Config); serr != nil {
			return nil, serr
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// SubscribedSet — return map[tool_name]bool buat efficient lookup di
// catalog endpoint (intersect with registry).
func (s *Store) SubscribedSet() (map[string]bool, error) {
	subs, err := s.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(subs))
	for _, ts := range subs {
		m[ts.ToolName] = true
	}
	return m, nil
}
