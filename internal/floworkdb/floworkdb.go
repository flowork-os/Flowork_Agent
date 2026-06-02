// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: SQLite store GLOBAL (owner-level). Audit pass — WAL +
//   busy_timeout + foreign_keys, SQL parameterized (? placeholder), table
//   name interpolasi cuma literal "kv"/"secrets" (bukan user input),
//   mu.Lock per method, rows.Close defer. E2E verified (auth/keys/wallet).
//
// Package floworkdb — SQLite store GLOBAL untuk data owner-level Flowork.
//
// Beda dari internal/agentdb (yang per-warga, terisolasi total di folder
// agent), floworkdb adalah SATU file `flowork.db` milik proses flowork-gui
// sendiri. Isinya hal-hal yang BUKAN milik warga manapun:
//
//   - kv      : config global (owner_name, dll)
//   - secrets : owner password hash + API key global (ETHERSCAN_API_KEY, dll)
//   - wallet_addresses : wallet crypto PERSONAL milik owner
//
// PENTING (doktrin isolasi): AI agent TIDAK menyimpan apa pun di sini.
// State warga tetap di `agents/<id>/workspace/state.db` (agentdb). floworkdb
// cuma untuk owner/host.
//
// Lokasi (portable, no-hardcode): env FLOWORK_DATA_DIR > ~/.flowork/ >
// /tmp/flowork/ — pola sama persis dengan loader.AgentsDir().
package floworkdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Path resolves lokasi flowork.db (global).
// Priority: FLOWORK_DATA_DIR env > ~/.flowork/flowork.db > /tmp/flowork/flowork.db
// (last resort biar headless smoke test tetap punya target writable).
func Path() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_DATA_DIR")); v != "" {
		return filepath.Join(v, "flowork.db")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".flowork", "flowork.db")
	}
	return filepath.Join("/tmp", "flowork", "flowork.db")
}

// Store — handle SQLite global.
type Store struct {
	mu   sync.Mutex
	db   *sql.DB
	Path string
}

// Open buka (atau bikin) DB file, ensure schema. Caller wajib Close()
// kecuali Shared() (singleton lifetime = proses).
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db parent: %w", err)
	}
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
		`CREATE TABLE IF NOT EXISTS secrets (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL DEFAULT ''
		) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS wallet_addresses (
			chain_id INTEGER NOT NULL,
			address  TEXT NOT NULL,
			label    TEXT NOT NULL DEFAULT '',
			added_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (chain_id, address)
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}

// ── kv ──────────────────────────────────────────────────────────────────

// GetKV returns value for key (empty string if absent).
func (s *Store) GetKV(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOne("kv", k)
}

// SetKV upsert key/value.
func (s *Store) SetKV(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsert("kv", k, v)
}

// ── secrets ─────────────────────────────────────────────────────────────

// GetSecret returns secret value for key (empty string if absent).
func (s *Store) GetSecret(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOne("secrets", k)
}

// SetSecret upsert secret key/value.
func (s *Store) SetSecret(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsert("secrets", k, v)
}

// DeleteSecret hapus 1 secret key.
func (s *Store) DeleteSecret(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM secrets WHERE k=?`, k)
	return err
}

// ListSecretKeys returns all secret keys (NOT values — caller masks).
func (s *Store) ListSecretKeys() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT k FROM secrets ORDER BY k`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if serr := rows.Scan(&k); serr != nil {
			return nil, serr
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// AllSecrets returns the full secret map. Used ONLY at boot to inject into
// env (os.Setenv). Never expose over HTTP.
func (s *Store) AllSecrets() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT k, v FROM secrets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if serr := rows.Scan(&k, &v); serr != nil {
			return nil, serr
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ── wallet_addresses (personal owner) ─────────────────────────────────────

// WalletAddress mirrors wallet_addresses row.
type WalletAddress struct {
	ChainID int    `json:"chain_id"`
	Address string `json:"address"`
	Label   string `json:"label"`
	AddedAt string `json:"added_at"`
}

// AddWalletAddress upsert (chain_id, address) with label.
func (s *Store) AddWalletAddress(chainID int, address, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	_, err := s.db.Exec(
		`DELETE FROM wallet_addresses WHERE chain_id = ? AND address = ?`,
		chainID, strings.TrimSpace(address),
	)
	return err
}

// ListWalletAddresses returns all owner personal wallet rows.
func (s *Store) ListWalletAddresses() ([]WalletAddress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT chain_id, address, label, added_at FROM wallet_addresses ORDER BY chain_id, address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WalletAddress{}
	for rows.Next() {
		var wa WalletAddress
		if serr := rows.Scan(&wa.ChainID, &wa.Address, &wa.Label, &wa.AddedAt); serr != nil {
			return nil, serr
		}
		out = append(out, wa)
	}
	return out, rows.Err()
}

// ── internal helpers ──────────────────────────────────────────────────────
// Table name di-interpolasi tapi callers controlled (literal "kv"/"secrets"
// hardcoded, bukan user input) — sama persis pola agentdb.

func (s *Store) getOne(table, k string) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT v FROM "+table+" WHERE k = ?", k).Scan(&v) // scanner:ignore — table = literal hardcoded (kv/secrets), key parameterized (?)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) upsert(table, k, v string) error {
	q := fmt.Sprintf("INSERT INTO %s(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v", table) // scanner:ignore — table = literal hardcoded (kv/secrets), value parameterized (?)
	_, err := s.db.Exec(q, k, v)
	return err
}

// ── singleton ─────────────────────────────────────────────────────────────

var (
	sharedMu  sync.Mutex
	sharedDB  *Store
	sharedErr error
)

// Shared returns the lazy process-wide store (opened once at first call).
func Shared() (*Store, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedDB == nil && sharedErr == nil {
		sharedDB, sharedErr = Open(Path())
	}
	return sharedDB, sharedErr
}
