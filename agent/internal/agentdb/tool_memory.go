// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

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
