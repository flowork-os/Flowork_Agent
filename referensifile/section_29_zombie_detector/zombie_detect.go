// Package workspacefs — zombie_detect.go: B-5 + B-14 fix.
//
// Detect zombie workspace folder: stale (no recent activity) + boilerplate
// README + roadmap kosong. REPORT-ONLY — Ayah keputusan hapus manual.
//
// Heuristic:
//   - Newest mtime di subtree > zombieAgeDays
//   - README.md size < zombieReadmeMin (boilerplate)
//   - Daily roadmap kosong / hanya skeleton
package workspacefs

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	zombieAgeDays    = 30
	zombieReadmeMin  = 600
	zombieDailyMin   = 300
)

// ZombieReport tentang satu workspace yang dicurigai zombie.
type ZombieReport struct {
	Name          string
	NewestMtime   time.Time
	StaleDays     int
	ReadmeSize    int64
	DailyTotal    int
	DailyStubs    int
	BoilerReadme  bool
	Reason        string
}

// DetectZombieWorkspaces scan workspaces/ root, return list folder yang
// kemungkinan zombie. REPORT-ONLY — caller decide action.
func DetectZombieWorkspaces(workspaceRoot string) ([]ZombieReport, error) {
	wsRoot := filepath.Join(workspaceRoot, "workspaces")
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -zombieAgeDays)
	var reports []ZombieReport

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		taskRoot := filepath.Join(wsRoot, e.Name())
		report := ZombieReport{Name: e.Name()}

		newest := time.Time{}
		_ = filepath.Walk(taskRoot, func(p string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info == nil {
				return nil
			}
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
			return nil
		})
		report.NewestMtime = newest
		if !newest.IsZero() {
			report.StaleDays = int(time.Since(newest).Hours() / 24)
		}

		readmePath := filepath.Join(taskRoot, "README.md")
		if st, err := os.Stat(readmePath); err == nil {
			report.ReadmeSize = st.Size()
			report.BoilerReadme = st.Size() < zombieReadmeMin
		}

		dailyDir := filepath.Join(taskRoot, "roadmap", "daily")
		if dailies, err := os.ReadDir(dailyDir); err == nil {
			for _, d := range dailies {
				if !strings.HasSuffix(d.Name(), ".md") {
					continue
				}
				report.DailyTotal++
				if st, err := os.Stat(filepath.Join(dailyDir, d.Name())); err == nil {
					if st.Size() < zombieDailyMin {
						report.DailyStubs++
					}
				}
			}
		}

		var reasons []string
		if !newest.IsZero() && newest.Before(cutoff) {
			reasons = append(reasons, "stale_30d")
		}
		if report.BoilerReadme {
			reasons = append(reasons, "boilerplate_readme")
		}
		if report.DailyTotal > 0 && report.DailyStubs == report.DailyTotal {
			reasons = append(reasons, "all_daily_stub")
		}
		if len(reasons) >= 2 {
			report.Reason = strings.Join(reasons, "+")
			reports = append(reports, report)
		}
	}

	return reports, nil
}
