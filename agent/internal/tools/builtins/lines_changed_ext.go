// lines_changed_ext.go — SIBLING ext (⚠️ FROZEN 2026-07-02 seizin owner — stabil+live): lacak BARIS DIUBAH
// (added/removed) tiap agent ngedit file (file_write/edit) — buat metrik "cost"
// lines-changed per-sesi (roadmap GUI). Pola interceptor sama kayak file_checkpoint
// (Before, best-effort, NOL sentuh frozen). Akumulasi in-memory per-agent (reset tiap
// boot = per-sesi). Endpoint /api/edits/stats (didaftar feature_edits_stats.go).
// 📄 Dok: lock/lines-changed.md
package builtins

import (
	"context"
	"os"
	"strings"
	"sync"

	"flowork-gui/internal/tools"
)

func init() { tools.RegisterInterceptor(linesChangedInterceptor{}) }

// editStat — akumulasi per agent (sejak boot = sesi).
type editStat struct {
	Added   int            `json:"added"`
	Removed int            `json:"removed"`
	Edits   int            `json:"edits"`
	Files   map[string]int `json:"-"`
}

var (
	editStatMu sync.Mutex
	editStats  = map[string]*editStat{} // agentID → stat
)

type linesChangedInterceptor struct{}

func (linesChangedInterceptor) Name() string { return "lines-changed" }

func (linesChangedInterceptor) Before(ctx context.Context, t tools.Tool, args map[string]any) error {
	var oldContent, newContent string
	switch t.Name() {
	case "file_write":
		abs, _, err := resolveFileArgs(ctx, args)
		if err != nil {
			return nil
		}
		oldContent = readFileBestEffort(abs)
		newContent, _ = args["content"].(string)
	case "edit":
		abs, _, err := resolveFileArgs(ctx, args)
		if err != nil {
			return nil
		}
		oldContent = readFileBestEffort(abs)
		oldS, _ := args["old_string"].(string)
		newS, _ := args["new_string"].(string)
		if oldS == "" || !strings.Contains(oldContent, oldS) {
			return nil // biar tool sendiri yang ngelapor; ga bisa rekonstruksi
		}
		if ra, _ := args["replace_all"].(bool); ra {
			newContent = strings.ReplaceAll(oldContent, oldS, newS)
		} else {
			newContent = strings.Replace(oldContent, oldS, newS, 1)
		}
	default:
		return nil // tool lain: ga dihitung
	}

	added, removed := lineDiffStat(oldContent, newContent)
	if added == 0 && removed == 0 {
		return nil
	}
	agent := tools.FromAgent(ctx)
	if agent == "" {
		agent = "(unknown)"
	}
	rel := ""
	if fp, ok := args["file_path"].(string); ok {
		rel = fp
	}

	editStatMu.Lock()
	s := editStats[agent]
	if s == nil {
		s = &editStat{Files: map[string]int{}}
		editStats[agent] = s
	}
	s.Added += added
	s.Removed += removed
	s.Edits++
	if rel != "" {
		s.Files[rel]++
	}
	editStatMu.Unlock()
	return nil // NON-blocking: metrik ga boleh ganggu tulisan (best-effort)
}

func readFileBestEffort(abs string) string {
	b, err := os.ReadFile(abs)
	if err != nil || len(b) > 4<<20 {
		return ""
	}
	return string(b)
}

// EditStatsSnapshot — dipakai endpoint (package main): total + per-agent sejak boot.
func EditStatsSnapshot() (total map[string]int, perAgent map[string]map[string]int) {
	editStatMu.Lock()
	defer editStatMu.Unlock()
	total = map[string]int{"added": 0, "removed": 0, "edits": 0, "files": 0}
	perAgent = map[string]map[string]int{}
	for ag, s := range editStats {
		total["added"] += s.Added
		total["removed"] += s.Removed
		total["edits"] += s.Edits
		total["files"] += len(s.Files)
		perAgent[ag] = map[string]int{"added": s.Added, "removed": s.Removed, "edits": s.Edits, "files": len(s.Files)}
	}
	return total, perAgent
}

// lineDiffStat — added/removed baris via LCS (akurat). Fallback net-count buat file
// besar (>4000 baris gabungan) biar ga O(n*m) berat.
func lineDiffStat(oldC, newC string) (added, removed int) {
	if oldC == newC {
		return 0, 0
	}
	oldL := splitLinesNoTrailEmpty(oldC)
	newL := splitLinesNoTrailEmpty(newC)
	if len(oldL) == 0 && len(newL) == 0 {
		return 0, 0
	}
	if len(oldL)+len(newL) > 4000 {
		d := len(newL) - len(oldL)
		if d >= 0 {
			return d, 0
		}
		return 0, -d
	}
	lcs := lcsLen(oldL, newL)
	return len(newL) - lcs, len(oldL) - lcs
}

func splitLinesNoTrailEmpty(s string) []string {
	if s == "" {
		return nil
	}
	l := strings.Split(s, "\n")
	if n := len(l); n > 0 && l[n-1] == "" {
		l = l[:n-1] // buang baris kosong dari trailing '\n'
	}
	return l
}

func lcsLen(a, b []string) int {
	dp := make([]int, len(b)+1)
	for i := len(a) - 1; i >= 0; i-- {
		prev := 0 // dp[i+1][j+1]
		for j := len(b) - 1; j >= 0; j-- {
			cur := dp[j]
			if a[i] == b[j] {
				dp[j] = prev + 1
			} else if dp[j+1] > dp[j] {
				dp[j] = dp[j+1]
			}
			prev = cur
		}
	}
	return dp[0]
}
