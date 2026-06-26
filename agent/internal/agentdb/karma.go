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

type Karma struct {
	MetricKey   string  `json:"metric_key"`
	MetricValue float64 `json:"metric_value"`
	MetricCount int64   `json:"metric_count"`
	UpdatedAt   string  `json:"updated_at"`
}

func (s *Store) IncrementKarma(key string, delta float64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return 0, fmt.Errorf("metric_key required")
	}
	const maxDelta = 1e9
	if delta > maxDelta || delta < -maxDelta {
		return 0, fmt.Errorf("delta out of range (|delta| > %g)", maxDelta)
	}

	ts := time.Now().UTC().Format(time.RFC3339)

	var current float64
	if err := s.db.QueryRow(
		`INSERT INTO karma_self(metric_key, metric_value, metric_count, updated_at)
		 VALUES(?, ?, 0, ?)
		 ON CONFLICT(metric_key) DO UPDATE SET
		     metric_value = karma_self.metric_value + excluded.metric_value,
		     updated_at = excluded.updated_at
		 RETURNING metric_value`,
		key, delta, ts,
	).Scan(&current); err != nil {
		return 0, fmt.Errorf("upsert karma: %w", err)
	}
	return current, nil
}

func (s *Store) AverageUpdateKarma(key string, value float64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return 0, fmt.Errorf("metric_key required")
	}
	const maxValue = 1e9
	if value < 0 || value > maxValue {
		return 0, fmt.Errorf("value out of range (0 ≤ value ≤ %g)", maxValue)
	}

	ts := time.Now().UTC().Format(time.RFC3339)

	var newAvg float64
	if err := s.db.QueryRow(
		`INSERT INTO karma_self(metric_key, metric_value, metric_count, updated_at)
		 VALUES(?, ?, 1, ?)
		 ON CONFLICT(metric_key) DO UPDATE SET
		     metric_value = (karma_self.metric_value * karma_self.metric_count + excluded.metric_value) / (karma_self.metric_count + 1),
		     metric_count = karma_self.metric_count + 1,
		     updated_at   = excluded.updated_at
		 RETURNING metric_value`,
		key, value, ts,
	).Scan(&newAvg); err != nil {
		return 0, fmt.Errorf("upsert avg karma: %w", err)
	}
	return newAvg, nil
}

func (s *Store) GetKarma(key string) (Karma, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return Karma{}, fmt.Errorf("metric_key required")
	}

	var k Karma
	err := s.db.QueryRow(
		`SELECT metric_key, metric_value, metric_count, updated_at
		 FROM karma_self WHERE metric_key = ?`, key,
	).Scan(&k.MetricKey, &k.MetricValue, &k.MetricCount, &k.UpdatedAt)
	if err == sql.ErrNoRows {
		return Karma{MetricKey: key}, nil
	}
	if err != nil {
		return Karma{}, fmt.Errorf("get karma: %w", err)
	}
	return k, nil
}

func (s *Store) ListKarma() ([]Karma, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		`SELECT metric_key, metric_value, metric_count, updated_at
		 FROM karma_self ORDER BY updated_at DESC LIMIT 100`,
	)
	if err != nil {
		return nil, fmt.Errorf("list karma: %w", err)
	}
	defer rows.Close()

	var out []Karma
	for rows.Next() {
		var k Karma
		if err := rows.Scan(&k.MetricKey, &k.MetricValue, &k.MetricCount, &k.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
