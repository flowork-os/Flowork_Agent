// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 1 (Episodic Interactions) DONE + adversarial-audit passed
//   (capability gate state:write, hold-lock through Open+Log, RFC3339 explicit
//   timestamp, 8KB content cap). API stable: LogInteraction/ListInteractions/
//   PruneInteractions/CountInteractions. Future Section 8 (Retention) extend
//   via NEW function di file lain — JANGAN ubah ini tanpa approval.
//
// interactions.go — Section 1 roadmap: Episodic interactions per-warga.
//
// PURPOSE:
//   Log tiap interaksi warga (Telegram in/out, RPC call, scheduler fire, dst.)
//   ke tabel `interactions` di state.db. Bukan untuk LLM context inject —
//   anti over-prompt. Tujuan: audit, recall manual via UI, analytics.
//
// ⚠️ OVER-PROMPT WARNING (per standar_ai_agent.md section 11):
//   JANGAN auto-inject interactions ke system prompt. Akses HANYA via
//   tool call (`memory_get` future tool) atau API endpoint untuk dashboard.

package agentdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Interaction — satu row di tabel `interactions`.
type Interaction struct {
	ID         int64          `json:"id"`
	Channel    string         `json:"channel"`     // 'telegram' | 'rpc' | 'cron' | 'schedule'
	Direction  string         `json:"direction"`   // 'in' | 'out'
	Actor      string         `json:"actor"`       // chat_id, caller_id, scheduler_id
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata"`    // message_id, group_id, model used
	OccurredAt string         `json:"occurred_at"` // ISO timestamp
}

// LogInteraction insert satu row. Idempotent terhadap timestamp — caller
// tidak set ID, SQLite autoincrement. Metadata di-marshal ke JSON.
//
// Content truncated kalau > 8KB (safety against bloat dari long payload).
func (s *Store) LogInteraction(channel, direction, actor, content string, metadata map[string]any) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channel == "" || direction == "" {
		return 0, fmt.Errorf("channel + direction required")
	}
	if direction != "in" && direction != "out" {
		return 0, fmt.Errorf("direction must be 'in' or 'out'")
	}

	// Hard cap content size — anti accidental dump big payload (mis. file content).
	const maxContentBytes = 8 * 1024
	if len(content) > maxContentBytes {
		content = content[:maxContentBytes] + "…[truncated]"
	}

	var metaJSON string
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err == nil {
			metaJSON = string(b)
		}
	}
	if metaJSON == "" {
		metaJSON = "{}"
	}

	// Timestamp explicit RFC3339 UTC supaya format konsisten dengan
	// PruneInteractions cutoff (audit fix: lexicographic compare di SQLite
	// rusak kalau insert pakai DEFAULT `YYYY-MM-DD HH:MM:SS` lalu prune
	// pakai RFC3339 dengan T+Z — format beda → comparison salah).
	ts := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO interactions(channel, direction, actor, content, metadata, occurred_at) VALUES(?, ?, ?, ?, ?, ?)`,
		channel, direction, actor, content, metaJSON, ts,
	)
	if err != nil {
		return 0, fmt.Errorf("insert interaction: %w", err)
	}
	return res.LastInsertId()
}

// ListInteractions — paginated list. Filter optional: channel, actor.
// Limit default 50, max 500 (bounded supaya ngga ke-DOS).
func (s *Store) ListInteractions(channel, actor string, limit int) ([]Interaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `SELECT id, channel, direction, actor, content, metadata, occurred_at
	          FROM interactions WHERE deleted_at IS NULL`
	args := []any{}
	if channel != "" {
		query += ` AND channel = ?`
		args = append(args, channel)
	}
	if actor != "" {
		query += ` AND actor = ?`
		args = append(args, actor)
	}
	query += ` ORDER BY occurred_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query interactions: %w", err)
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		var metaRaw string
		if err := rows.Scan(&it.ID, &it.Channel, &it.Direction, &it.Actor, &it.Content, &metaRaw, &it.OccurredAt); err != nil {
			return nil, err
		}
		if metaRaw != "" && metaRaw != "{}" {
			_ = json.Unmarshal([]byte(metaRaw), &it.Metadata)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// PruneInteractions — soft-delete row yang occurred_at lebih lama dari
// olderThan (e.g. 30 days). Return count deleted. Hard-delete kemudian
// jalan via retention cron (section 8 roadmap, future).
func (s *Store) PruneInteractions(olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE interactions SET deleted_at = CURRENT_TIMESTAMP
		 WHERE deleted_at IS NULL AND occurred_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune interactions: %w", err)
	}
	return res.RowsAffected()
}

// CountInteractions — total non-deleted row. Buat metric / dashboard.
func (s *Store) CountInteractions() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM interactions WHERE deleted_at IS NULL`).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
