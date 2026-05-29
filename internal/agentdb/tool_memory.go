// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 11 phase 1a (tool memory KV) DONE. API stable:
//   GetToolMemory (return value + found bool), SetToolMemory (upsert
//   ON CONFLICT, 32KB value cap + 256 byte key cap), DelToolMemory
//   (DESTRUCTIVE physical remove — schema NO deleted_at), ListToolMemoryKeys
//   (cap 100 keys-only anti over-prompt). Future fact_remember tools
//   bisa pakai same helpers atau new dedicated table.
//
// tool_memory.go — Section 11 phase 1a: simple KV store buat tool
// memory_get/set/delete. Separate dari existing `kv` table supaya
// ownership tool memory terisolasi.
//
// SCHEMA: tool_memory(k TEXT PRIMARY KEY, v TEXT, updated_at TEXT).
//
// ⚠️ Anti over-prompt: tool_memory list endpoint cap 100 row. JANGAN
// auto-inject ke chat — caller eksplisit panggil memory_get(key).

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

// GetToolMemory — single read. Return zero string + ok=false kalau ngga ada.
func (s *Store) GetToolMemory(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return "", false, fmt.Errorf("key required")
	}
	var v string
	err := s.db.QueryRow(`SELECT v FROM tool_memory WHERE k = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get tool_memory: %w", err)
	}
	return v, true, nil
}

// SetToolMemory — upsert by PRIMARY KEY. Hard cap value 32KB.
func (s *Store) SetToolMemory(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return fmt.Errorf("key required")
	}
	const maxValBytes = 32 * 1024
	if len(value) > maxValBytes {
		value = value[:maxValBytes] + "…[truncated]"
	}
	const maxKeyBytes = 256
	if len(key) > maxKeyBytes {
		return fmt.Errorf("key too long (max %d bytes)", maxKeyBytes)
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO tool_memory(k, v, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(k) DO UPDATE SET v = excluded.v, updated_at = excluded.updated_at`,
		key, value, ts,
	)
	if err != nil {
		return fmt.Errorf("set tool_memory: %w", err)
	}
	return nil
}

// DelToolMemory — DESTRUCTIVE physical remove. Return rows affected.
func (s *Store) DelToolMemory(key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return 0, fmt.Errorf("key required")
	}
	res, err := s.db.Exec(`DELETE FROM tool_memory WHERE k = ?`, key)
	if err != nil {
		return 0, fmt.Errorf("delete tool_memory: %w", err)
	}
	return res.RowsAffected()
}

// ListToolMemoryKeys — return sorted keys, cap 100 (anti over-prompt).
// Body value tidak di-include — caller pull per-key.
func (s *Store) ListToolMemoryKeys() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT k FROM tool_memory ORDER BY k ASC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("list tool_memory: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
