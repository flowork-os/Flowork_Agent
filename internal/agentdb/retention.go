// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 8 (Retention policy + cron) DONE + adversarial-audit
//   passed (C1 zero-window defense + C2 tx-wrap hard-delete + I1 audit
//   trail ke decisions). API stable: RetentionWindows, DefaultRetention,
//   RetentionReport, PrunePromotedMistakes, HardDeleteSoftDeleted (atomic
//   tx), RunRetentionSweep (normalize windows, log via LogDecision).
//   minRetentionDuration=24h hard floor. Future config override (per-warga
//   custom windows) → tambah function/file baru, JANGAN modify.
//
// retention.go — Section 8 roadmap: Retention policy & soft-delete sweep.
//
// PURPOSE:
//   Tabel di state.db ngga boleh grow unbounded. retention.go orchestrate
//   2 phase cleanup:
//     1. Soft-delete (mark deleted_at) row yang lama lewat retention window
//     2. Hard-delete row yang sudah soft-deleted lebih dari grace period
//
// Schedule default per roadmap section 8:
//   - interactions       > 30 hari      → soft-delete (PruneInteractions)
//   - decisions          > 90 hari      → soft-delete (PruneDecisions)
//   - mistakes (raw)     > 90 hari      → soft-delete (PruneMistakes — di mistakes.go locked)
//   - mistakes (promoted)> 180 hari     → soft-delete (PrunePromotedMistakes di file ini)
//   - soft-deleted rows  > 90 hari      → HARD delete (HardDeleteSoftDeleted)
//
// Tidak di-prune: workspace_meta (sumber-of-truth filesystem), karma_self
// (state perpetual), death_letter (legacy). Section 5/6 implement nanti.
//
// Cron caller: lihat kernelhost (per-agent sweep tiap 24h) atau admin
// trigger via POST /api/agents/retention/sweep?id=.

package agentdb

import (
	"fmt"
	"time"
)

// RetentionWindows — durasi tiap kategori. Default per roadmap section 8.
// Caller bisa override untuk testing.
type RetentionWindows struct {
	Interactions       time.Duration // default 30 hari
	Decisions          time.Duration // default 90 hari
	MistakesRaw        time.Duration // default 90 hari
	MistakesPromoted   time.Duration // default 180 hari
	HardDeleteGrace    time.Duration // default 90 hari (post soft-delete)
}

// DefaultRetention returns roadmap-section-8 defaults.
func DefaultRetention() RetentionWindows {
	return RetentionWindows{
		Interactions:     30 * 24 * time.Hour,
		Decisions:        90 * 24 * time.Hour,
		MistakesRaw:      90 * 24 * time.Hour,
		MistakesPromoted: 180 * 24 * time.Hour,
		HardDeleteGrace:  90 * 24 * time.Hour,
	}
}

// RetentionReport — hasil satu kali sweep run. Buat dashboard + decision log.
type RetentionReport struct {
	StartedAt              string `json:"started_at"`
	FinishedAt             string `json:"finished_at"`
	SoftDeletedInteractions int64 `json:"soft_deleted_interactions"`
	SoftDeletedDecisions    int64 `json:"soft_deleted_decisions"`
	SoftDeletedMistakesRaw  int64 `json:"soft_deleted_mistakes_raw"`
	SoftDeletedMistakesProm int64 `json:"soft_deleted_mistakes_promoted"`
	HardDeletedInteractions int64 `json:"hard_deleted_interactions"`
	HardDeletedDecisions    int64 `json:"hard_deleted_decisions"`
	HardDeletedMistakes     int64 `json:"hard_deleted_mistakes"`
	Errors                  []string `json:"errors,omitempty"`
}

// minRetentionDuration — guard supaya caller ngga accidentally pass
// duration kecil/zero yang ngehapus row yang baru bikin. 24 jam adalah
// minimum sane untuk retention sweep apapun.
const minRetentionDuration = 24 * time.Hour

// PrunePromotedMistakes — soft-delete row tier='promoted' yang last_hit_at
// lebih lama dari olderThan. Tier 'raw' di-handle PruneMistakes (mistakes.go
// locked). Tier 'reviewed' tidak di-prune — sakral (workflow-in-progress).
//
// Refuse run kalau olderThan < minRetentionDuration (24h) — defense
// against zero RetentionWindows{} accidentally dipassing.
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

// HardDeleteSoftDeleted — final cleanup. Hapus permanen row yang sudah
// soft-deleted lebih dari grace period. Cover 3 tabel: interactions,
// decisions, mistakes_local. Return total row dihapus per tabel.
//
// ⚠️ DESTRUCTIVE: row hilang permanen. Defense in depth:
//   - Refuse run kalau grace < 24h (cegah zero RetentionWindows{} hapus
//     row baru di-soft-delete detik lalu).
//   - Caller WAJIB pakai DefaultRetention() atau set explicit window > 24h.
//   - 3 DELETE atomic dalam transaction. Crash di tengah → rollback semua.
//     Tanpa transaction, ref_interaction_id di decisions bisa point ke
//     interactions yang udah ke-DELETE (silent orphan) — audit Section 3
//     cross-ref jadi rusak.
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
	tx = nil // sukses — skip rollback di defer
	return
}

// RunRetentionSweep — orchestrate full sweep: soft-delete (4 phase) +
// hard-delete (3 tabel). Aggregate report. Tidak short-circuit pada error
// — kumpulin semua di report.Errors.
//
// Defense: normalize windows — kalau caller pass zero / under-min, fallback
// ke DefaultRetention() values. Cegah accidental DELETE row baru.
//
// Caller cron loop ATAU admin manual trigger (POST endpoint).
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

	// Section 3 doctrine: log retention sweep ke decisions table sebagai
	// audit trail (kernel log.Printf hilang post-restart). Guard: skip
	// kalau ngga ada deletion + ngga ada error — reduce noise.
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
		// Best-effort: kalau LogDecision sendiri gagal, append ke report
		// (jangan crash sweep yang udah selesai).
		if _, lerr := s.LogDecision("retention_sweep",
			fmt.Sprintf("Retention sweep: %d row affected (soft+hard).", totalAffected),
			outcome, inputs, 0); lerr != nil {
			rep.Errors = append(rep.Errors, "log decision: "+lerr.Error())
		}
	}

	return rep
}
