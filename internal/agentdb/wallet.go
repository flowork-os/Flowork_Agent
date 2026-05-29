// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 21 phase 1 wallet schema. Lazy CREATE — schemas.go locked.
//   Phase 2 (snapshot rotation, top-N retention, foreign chain plug-in) →
//   tambah file baru.
//
// wallet.go — Section 21 phase 1: wallet_addresses + wallet_snapshots.

package agentdb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// WalletAddress mirrors wallet_addresses row.
type WalletAddress struct {
	ChainID int    `json:"chain_id"`
	Address string `json:"address"`
	Label   string `json:"label"`
	AddedAt string `json:"added_at"`
}

// WalletSnapshot mirrors wallet_snapshots row.
type WalletSnapshot struct {
	ID            int64   `json:"id"`
	TakenAt       string  `json:"taken_at"`
	TotalUSD      float64 `json:"total_usd"`
	PortfolioJSON string  `json:"portfolio_json"`
}

// ensureWalletSchema — lazy CREATE.
func (s *Store) ensureWalletSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS wallet_addresses (
		  chain_id INTEGER NOT NULL,
		  address  TEXT NOT NULL,
		  label    TEXT NOT NULL DEFAULT '',
		  added_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (chain_id, address)
		);
		CREATE TABLE IF NOT EXISTS wallet_snapshots (
		  id             INTEGER PRIMARY KEY AUTOINCREMENT,
		  taken_at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  total_usd      REAL NOT NULL DEFAULT 0,
		  portfolio_json TEXT NOT NULL DEFAULT '{}'
		);
		CREATE INDEX IF NOT EXISTS idx_wallet_snapshots_taken
		  ON wallet_snapshots(taken_at DESC);
	`)
	return err
}

// AddWalletAddress upsert (chain_id, address) with label.
func (s *Store) AddWalletAddress(chainID int, address, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletSchema(); err != nil {
		return err
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("address required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO wallet_addresses (chain_id, address, label, added_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(chain_id, address) DO UPDATE SET label = excluded.label`,
		chainID, address, label, now,
	)
	return err
}

// DeleteWalletAddress remove row.
func (s *Store) DeleteWalletAddress(chainID int, address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM wallet_addresses WHERE chain_id = ? AND address = ?`,
		chainID, address,
	)
	return err
}

// ListWalletAddresses returns all rows.
func (s *Store) ListWalletAddresses() ([]WalletAddress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT chain_id, address, label, added_at FROM wallet_addresses ORDER BY chain_id, address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WalletAddress{}
	for rows.Next() {
		var w WalletAddress
		if serr := rows.Scan(&w.ChainID, &w.Address, &w.Label, &w.AddedAt); serr != nil {
			return nil, serr
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// InsertWalletSnapshot — append snapshot. Caller (cron) panggil periodic.
func (s *Store) InsertWalletSnapshot(totalUSD float64, portfolioJSON string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletSchema(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO wallet_snapshots (taken_at, total_usd, portfolio_json) VALUES (?, ?, ?)`,
		now, totalUSD, portfolioJSON,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListWalletSnapshots — paginated DESC by id.
func (s *Store) ListWalletSnapshots(limit int) ([]WalletSnapshot, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 500 {
		limit = 500
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWalletSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, taken_at, total_usd, portfolio_json
		 FROM wallet_snapshots
		 ORDER BY id DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WalletSnapshot{}
	for rows.Next() {
		var w WalletSnapshot
		var pj sql.NullString
		if serr := rows.Scan(&w.ID, &w.TakenAt, &w.TotalUSD, &pj); serr != nil {
			return nil, serr
		}
		if pj.Valid {
			w.PortfolioJSON = pj.String
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
