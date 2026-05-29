// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 3 (Decisions log) DONE + adversarial-audit passed.
//   API stable: LogDecision (return ID), ListDecisions (type filter), Prune,
//   Count. RFC3339 timestamp explicit, 4KB rationale cap, 'pending' outcome
//   default. Section 8 (Retention) extend via new function di file lain —
//   JANGAN ubah ini tanpa approval.
//
// decisions.go — Section 3 roadmap: Decisions log per-warga.
//
// PURPOSE:
//   Audit trail keputusan non-trivial warga (mis. pilih model, drop chat
//   unauthorized, LLM fail → fallback, tool pick). Bukan untuk LLM context
//   inject — anti over-prompt. Tujuan: debugging, accountability, training
//   future warga via pattern analysis.
//
// ⚠️ OVER-PROMPT WARNING (per standar_ai_agent.md section 11):
//   JANGAN auto-inject decisions ke system prompt. Akses HANYA via tool
//   call (`decisions_recall` future tool) atau API endpoint untuk dashboard.

package agentdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Decision — satu row di tabel `decisions`.
type Decision struct {
	ID               int64          `json:"id"`
	DecisionType     string         `json:"decision_type"`        // 'model_choice' | 'skip_task' | 'escalate' | 'tool_pick' | dst
	Rationale        string         `json:"rationale"`
	Inputs           map[string]any `json:"inputs"`               // JSON: konteks input
	Outcome          string         `json:"outcome"`              // 'success' | 'fail' | 'pending'
	RefInteractionID int64          `json:"ref_interaction_id"`   // 0 = tidak link
	OccurredAt       string         `json:"occurred_at"`          // ISO timestamp
}

// LogDecision insert satu row keputusan. Idempotent terhadap timestamp —
// caller tidak set ID, SQLite autoincrement. Inputs di-marshal ke JSON.
//
// Rationale truncated kalau > 4KB (decisions rationale lebih pendek dari
// content interaction; 4KB sudah generous).
//
// Anti-spoof: caller wajib lewat host capability (kernel inject pluginID
// dari ctx). DB layer cuma trust caller.
func (s *Store) LogDecision(decisionType, rationale, outcome string, inputs map[string]any, refInteractionID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if decisionType == "" || rationale == "" {
		return 0, fmt.Errorf("decision_type + rationale required")
	}

	// Hard cap rationale supaya tidak bloat.
	const maxRationaleBytes = 4 * 1024
	if len(rationale) > maxRationaleBytes {
		rationale = rationale[:maxRationaleBytes] + "…[truncated]"
	}

	var inputsJSON string
	if len(inputs) > 0 {
		b, err := json.Marshal(inputs)
		if err == nil {
			inputsJSON = string(b)
		}
	}
	if inputsJSON == "" {
		inputsJSON = "{}"
	}

	// Outcome empty di-treat sebagai 'pending' supaya consistent. Caller bisa
	// override saat decision selesai eksekusi (via separate update — defer
	// sampai ada use case follow-up).
	if outcome == "" {
		outcome = "pending"
	}

	// Timestamp explicit RFC3339 UTC supaya konsisten dengan PruneDecisions
	// cutoff (sama isu sebagai LogInteraction — lihat audit notes Section 1).
	ts := time.Now().UTC().Format(time.RFC3339)

	// NULL kalau ref tidak set (0). FK constraint defer — kalau tabel
	// interactions ke-prune dan decision masih reference id lama, tetap row
	// decision valid (FK ngga ON DELETE CASCADE; soft-delete).
	var refArg any
	if refInteractionID > 0 {
		refArg = refInteractionID
	}

	res, err := s.db.Exec(
		`INSERT INTO decisions(decision_type, rationale, inputs, outcome, ref_interaction_id, occurred_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		decisionType, rationale, inputsJSON, outcome, refArg, ts,
	)
	if err != nil {
		return 0, fmt.Errorf("insert decision: %w", err)
	}
	return res.LastInsertId()
}

// ListDecisions — paginated list. Filter optional: decision_type.
// Limit default 50, max 500 (bounded supaya tidak ke-DOS).
func (s *Store) ListDecisions(decisionType string, limit int) ([]Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `SELECT id, decision_type, rationale, inputs, outcome,
	                 COALESCE(ref_interaction_id, 0), occurred_at
	          FROM decisions WHERE deleted_at IS NULL`
	args := []any{}
	if decisionType != "" {
		query += ` AND decision_type = ?`
		args = append(args, decisionType)
	}
	query += ` ORDER BY occurred_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query decisions: %w", err)
	}
	defer rows.Close()

	var out []Decision
	for rows.Next() {
		var d Decision
		var inputsRaw string
		if err := rows.Scan(&d.ID, &d.DecisionType, &d.Rationale, &inputsRaw,
			&d.Outcome, &d.RefInteractionID, &d.OccurredAt); err != nil {
			return nil, err
		}
		if inputsRaw != "" && inputsRaw != "{}" {
			_ = json.Unmarshal([]byte(inputsRaw), &d.Inputs)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PruneDecisions — soft-delete row yang occurred_at lebih lama dari
// olderThan (e.g. 90 days). Return count deleted. Hard-delete kemudian
// jalan via retention cron (section 8 roadmap).
func (s *Store) PruneDecisions(olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE decisions SET deleted_at = CURRENT_TIMESTAMP
		 WHERE deleted_at IS NULL AND occurred_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune decisions: %w", err)
	}
	return res.RowsAffected()
}

// CountDecisions — total non-deleted row. Buat metric / dashboard.
func (s *Store) CountDecisions() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM decisions WHERE deleted_at IS NULL`).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
