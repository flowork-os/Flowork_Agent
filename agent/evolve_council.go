// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-16 (autonomous sprint A1 dewan).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner).
// 2026-06-21 (owner-approved, AI-IN-AGENT cleanup): HAPUS evolveCouncilJudge() inline =
//   DEAD CODE — disuperseded evolveCouncilJudgeViaGroup() (DEWAN via 5 AGENT, model GUI)
//   yang wired di main.go:957. Inline cuma ke-sebut di KOMENTAR, ga pernah dipanggil, +
//   dia pakai coderModel("") (global, melanggar "kebenaran = setting per-agent"). +buang
//   const evolvePillarDesc (cuma dipakai fungsi mati itu) + import dirampingin. parseJudgeVote
//   DIPERTAHANKAN (dipakai evolve_council_group.go). Re-locked.
//
// evolve_council.go — helper parse vote dewan adversarial. Logika debat 5-suara udah pindah
// ke evolve_council_group.go (otak di AGENT, model GUI per-agent — owner 2026-06-20).
package main

import (
	"strconv"
	"strings"
)

// parseJudgeVote — ambil DECISION/SCORE/REASON dari balasan hakim (toleran).
// Dipakai evolve_council_group.go (dewan via grup self-evolution: 5 agent).
func parseJudgeVote(s string) (decision string, score int, reason string) {
	decision = "stage" // default konservatif kalau gagal parse
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		up := strings.ToUpper(t)
		switch {
		case strings.HasPrefix(up, "DECISION:"):
			d := strings.ToLower(strings.TrimSpace(t[len("DECISION:"):]))
			for _, k := range []string{"approve", "reject", "stage"} {
				if strings.Contains(d, k) {
					decision = k
				}
			}
		case strings.HasPrefix(up, "SCORE:"):
			fields := strings.FieldsFunc(t[len("SCORE:"):], func(r rune) bool { return r < '0' || r > '9' })
			if len(fields) > 0 {
				if n, err := strconv.Atoi(fields[0]); err == nil {
					score = n
				}
			}
		case strings.HasPrefix(up, "REASON:"):
			reason = strings.TrimSpace(t[len("REASON:"):])
		}
	}
	return decision, score, reason
}
