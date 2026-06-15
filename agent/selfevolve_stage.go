// === LOCKED FILE (soft) === Status: STABLE — DO NOT MODIFY without owner approval (LOCKED ≠ FREEZE).
// Owner: Aola Sahidin (Mr.Dev) · Locked 2026-06-16. Reason: R7 Milestone C stage committer. VERIFIED
// E2E: owner approve staged diff → commit isi yg DIREVIEW (8d99779 by "Flowork Evolusi") + reject path.
//
// selfevolve_stage.go — R7 fase-2b Milestone C: committer buat STAGE review (owner approve).
// Owner APPROVE staged diff → commit ISI yg DIREVIEW (bukan re-codegen) ke local main + maybe
// push (kalau enabled). Reuse evolveCommitFile + evolveMaybePush (engine core-apply, LOCKED).
// Decoupling: agentmgr (EvolveStageActionHandler) nyimpen lifecycle, main nyuntik kemampuan commit.

package main

import (
	"context"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
)

// evolveStageCommitter — di-inject ke EvolveStageActionHandler. Commit isi stage (yg owner
// review) ke local main, lalu push kalau auto-push enabled. Manual-approve = owner in-the-loop.
func evolveStageCommitter() agentmgr.EvolveStageCommitter {
	return func(ctx context.Context, st agentdb.EvolveStage) (map[string]any, error) {
		root, err := evolveRepoRoot()
		if err != nil {
			return nil, err
		}
		msg := "evolve(approved): tambah " + st.TargetFile + " — owner-approved staged diff"
		hash, cerr := evolveCommitFile(ctx, root, st.TargetFile, st.Content, msg)
		if cerr != nil {
			return nil, cerr
		}
		pushed, pnote := evolveMaybePush(ctx, root)
		return map[string]any{
			"hash":   hash,
			"pushed": pushed,
			"note":   "commit lokal " + hash + " (owner-approved). Push: " + pnote,
		}, nil
	}
}
