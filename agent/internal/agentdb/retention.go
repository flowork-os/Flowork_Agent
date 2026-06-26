// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"fmt"
	"time"
)

type RetentionWindows struct {
	Interactions     time.Duration
	Decisions        time.Duration
	MistakesRaw      time.Duration
	MistakesPromoted time.Duration
	HardDeleteGrace  time.Duration
}

func DefaultRetention() RetentionWindows {
	return RetentionWindows{
		Interactions:     30 * 24 * time.Hour,
		Decisions:        90 * 24 * time.Hour,
		MistakesRaw:      90 * 24 * time.Hour,
		MistakesPromoted: 180 * 24 * time.Hour,
		HardDeleteGrace:  90 * 24 * time.Hour,
	}
}

type RetentionReport struct {
	StartedAt               string   `json:"started_at"`
	FinishedAt              string   `json:"finished_at"`
	SoftDeletedInteractions int64    `json:"soft_deleted_interactions"`
	SoftDeletedDecisions    int64    `json:"soft_deleted_decisions"`
	SoftDeletedMistakesRaw  int64    `json:"soft_deleted_mistakes_raw"`
	SoftDeletedMistakesProm int64    `json:"soft_deleted_mistakes_promoted"`
	HardDeletedInteractions int64    `json:"hard_deleted_interactions"`
	HardDeletedDecisions    int64    `json:"hard_deleted_decisions"`
	HardDeletedMistakes     int64    `json:"hard_deleted_mistakes"`
	Errors                  []string `json:"errors,omitempty"`
}

const minRetentionDuration = 24 * time.Hour

func (s *Store) PrunePromotedMistakes(olderThan time.Duration) (int64, error) {
	if olderThan < minRetentionDuration {
		return 0, fmt.Errorf("retention duration too small (got %s, min %s)", olderThan, minRetentionDuration)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE mistakes_local SET deleted_at = CURRENT_TIMESTAMP, deleted_by = 'retention-cron'
		 WHERE deleted_at IS NULL AND tier = 'promoted' AND last_hit_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune promoted mistakes: %w", err)
	}
	return res.RowsAffected()
}

func (s *Store) HardDeleteSoftDeleted(grace time.Duration) (interactions, decisions, mistakes int64, err error) {
	if grace < minRetentionDuration {
		err = fmt.Errorf("hard-delete grace too small (got %s, min %s)", grace, minRetentionDuration)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-grace).Format(time.RFC3339)

	tx, txErr := s.db.Begin()
	if txErr != nil {
		err = fmt.Errorf("begin tx: %w", txErr)
		return
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	r1, e1 := tx.Exec(
		`DELETE FROM interactions WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		cutoff,
	)
	if e1 != nil {
		err = fmt.Errorf("hard delete interactions: %w", e1)
		return
	}
	interactions, _ = r1.RowsAffected()

	r2, e2 := tx.Exec(
		`DELETE FROM decisions WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		cutoff,
	)
	if e2 != nil {
		err = fmt.Errorf("hard delete decisions: %w", e2)
		return
	}
	decisions, _ = r2.RowsAffected()

	r3, e3 := tx.Exec(
		`DELETE FROM mistakes_local WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		cutoff,
	)
	if e3 != nil {
		err = fmt.Errorf("hard delete mistakes: %w", e3)
		return
	}
	mistakes, _ = r3.RowsAffected()

	if cerr := tx.Commit(); cerr != nil {
		err = fmt.Errorf("commit tx: %w", cerr)
		return
	}
	tx = nil
	return
}

func (s *Store) RunRetentionSweep(w RetentionWindows) RetentionReport {
	def := DefaultRetention()
	if w.Interactions < minRetentionDuration {
		w.Interactions = def.Interactions
	}
	if w.Decisions < minRetentionDuration {
		w.Decisions = def.Decisions
	}
	if w.MistakesRaw < minRetentionDuration {
		w.MistakesRaw = def.MistakesRaw
	}
	if w.MistakesPromoted < minRetentionDuration {
		w.MistakesPromoted = def.MistakesPromoted
	}
	if w.HardDeleteGrace < minRetentionDuration {
		w.HardDeleteGrace = def.HardDeleteGrace
	}

	rep := RetentionReport{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if n, err := s.PruneInteractions(w.Interactions); err != nil {
		rep.Errors = append(rep.Errors, "prune interactions: "+err.Error())
	} else {
		rep.SoftDeletedInteractions = n
	}

	if n, err := s.PruneDecisions(w.Decisions); err != nil {
		rep.Errors = append(rep.Errors, "prune decisions: "+err.Error())
	} else {
		rep.SoftDeletedDecisions = n
	}

	if n, err := s.PruneMistakes(w.MistakesRaw); err != nil {
		rep.Errors = append(rep.Errors, "prune mistakes raw: "+err.Error())
	} else {
		rep.SoftDeletedMistakesRaw = n
	}

	if n, err := s.PrunePromotedMistakes(w.MistakesPromoted); err != nil {
		rep.Errors = append(rep.Errors, "prune mistakes promoted: "+err.Error())
	} else {
		rep.SoftDeletedMistakesProm = n
	}

	hi, hd, hm, err := s.HardDeleteSoftDeleted(w.HardDeleteGrace)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
	rep.HardDeletedInteractions = hi
	rep.HardDeletedDecisions = hd
	rep.HardDeletedMistakes = hm

	rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)

	totalAffected := rep.SoftDeletedInteractions + rep.SoftDeletedDecisions +
		rep.SoftDeletedMistakesRaw + rep.SoftDeletedMistakesProm +
		rep.HardDeletedInteractions + rep.HardDeletedDecisions + rep.HardDeletedMistakes
	if totalAffected > 0 || len(rep.Errors) > 0 {
		outcome := "success"
		if len(rep.Errors) > 0 {
			outcome = "fail"
		}
		inputs := map[string]any{
			"soft_deleted_interactions":      rep.SoftDeletedInteractions,
			"soft_deleted_decisions":         rep.SoftDeletedDecisions,
			"soft_deleted_mistakes_raw":      rep.SoftDeletedMistakesRaw,
			"soft_deleted_mistakes_promoted": rep.SoftDeletedMistakesProm,
			"hard_deleted_interactions":      rep.HardDeletedInteractions,
			"hard_deleted_decisions":         rep.HardDeletedDecisions,
			"hard_deleted_mistakes":          rep.HardDeletedMistakes,
		}
		if len(rep.Errors) > 0 {
			inputs["errors"] = rep.Errors
		}

		if _, lerr := s.LogDecision("retention_sweep",
			fmt.Sprintf("Retention sweep: %d row affected (soft+hard).", totalAffected),
			outcome, inputs, 0); lerr != nil {
			rep.Errors = append(rep.Errors, "log decision: "+lerr.Error())
		}
	}

	return rep
}
