// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 5 (Karma self) DONE + adversarial-audit passed.
//   API stable: IncrementKarma (counter pattern via ON CONFLICT upsert),
//   AverageUpdateKarma (moving avg via atomic tx), GetKarma (zero+key
//   on absence), ListKarma (LIMIT 100). Hard cap 1e9 anti-runaway.
//   NO soft-delete (state perpetual, retention section 8 skip).
//   Future analytics extension (histogram, time-series) → tambah
//   function/file baru, JANGAN modify ini.
//
// karma.go — Section 5 roadmap: Karma self per-warga.
//
// PURPOSE:
//   Tiap warga track metric diri sendiri — success rate, fail count,
//   avg response time. Bukan ranking lintas-warga (itu router kalau
//   perlu), tapi self-improvement signal.
//
// SEMANTIC:
//   - IncrementKarma(key, delta): counter style. delta=1 increment,
//     bisa juga decrement (delta=-1). Tidak track moving avg.
//   - AverageUpdateKarma(key, value): moving average style — combine
//     current avg dengan new sample. metric_count++ tiap update.
//   - GetKarma(key): single read.
//   - ListKarma(): semua metric, untuk dashboard.
//
// State perpetual — NO soft-delete (lihat retention section 8 exclusion).
//
// ⚠️ OVER-PROMPT WARNING (standar section 11):
//   Karma value bisa di-inject ke persona sebagai 1-baris context
//   ("lo punya success rate 95%"), TAPI jangan stuff full metric list.

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

// Karma — satu metric row.
type Karma struct {
	MetricKey   string  `json:"metric_key"`
	MetricValue float64 `json:"metric_value"`
	MetricCount int64   `json:"metric_count"`
	UpdatedAt   string  `json:"updated_at"`
}

// IncrementKarma — counter pattern. Pakai untuk success_count, fail_count,
// dst. metric_count NGGA di-touch (tetap 0 untuk counter style).
//
// Audit fix C1: bungkus upsert + SELECT current dalam transaction supaya
// hasil current konsisten dengan caller's increment (cegah skew log walau
// concurrent guest call). modernc.org/sqlite mendukung `RETURNING` clause
// — gw pakai supaya satu query atomic.
//
// Hard cap |delta| > 1e9 → reject (anti runaway).
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

	// Single atomic UPSERT with RETURNING — value yang di-return = value
	// post-update (atau initial kalau fresh insert). modernc.org/sqlite
	// (v1.51.0) support RETURNING clause sejak SQLite 3.35.
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

// AverageUpdateKarma — moving average pattern. Pakai untuk avg_response_ms,
// avg_token_count, dst. Formula:
//
//	new_avg = (old_avg * old_count + new_value) / (old_count + 1)
//
// Audit fix C2: COMPUTE FORMULA DI DB LEVEL via SINGLE atomic UPSERT.
// Sebelumnya pakai SELECT + compute + UPSERT yang race-prone — 2 concurrent
// caller bisa baca oldCount sama → sample HILANG di overwrite. Sekarang:
//
//	INSERT VALUES(?, ?, 1, ts)
//	ON CONFLICT(metric_key) DO UPDATE SET
//	    metric_value = (metric_value * metric_count + ?) / (metric_count + 1),
//	    metric_count = metric_count + 1
//
// Atomic per SQLite — 2 concurrent caller serialize via writer lock,
// kedua sample tercatat.
//
// value boleh > 0; reject negative + extreme.
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

	// Single atomic UPSERT — formula compute di DB engine, no race.
	// RETURNING metric_value supaya caller dapet new_avg langsung.
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

// GetKarma — single metric read. Return zero Karma + err == nil kalau key
// belum ada (caller handle absence).
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

// ListKarma — semua metric, ordered by updated_at DESC. Bounded supaya
// ngga ke-DOS (cap 100 — karma ngga harusnya > beberapa puluh metric).
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
