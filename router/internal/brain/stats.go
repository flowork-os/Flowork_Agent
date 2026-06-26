// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"context"
	"os"
)

type WingCount struct {
	Wing  string `json:"wing"`
	Count int64  `json:"count"`
}

type Stats struct {
	Available bool        `json:"available"`
	Path      string      `json:"path"`
	SizeBytes int64       `json:"sizeBytes"`
	Drawers   int64       `json:"drawers"`
	Wings     []WingCount `json:"wings"`
	Skills    int         `json:"skills"`
}

func GetStats(ctx context.Context) Stats {
	st := Stats{Path: DBPath(), Skills: len(Skills())}
	if !Available() {
		return st
	}
	st.Available = true
	if info, err := os.Stat(st.Path); err == nil {
		st.SizeBytes = info.Size()
	}
	db, err := Open()
	if err != nil {
		return st
	}

	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drawers WHERE deleted_at IS NULL`).Scan(&st.Drawers)
	rows, err := db.QueryContext(ctx, `SELECT wing, COUNT(*) c FROM drawers
		WHERE deleted_at IS NULL GROUP BY wing ORDER BY c DESC LIMIT 12`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var w WingCount
			if err := rows.Scan(&w.Wing, &w.Count); err == nil {
				st.Wings = append(st.Wings, w)
			}
		}
	}
	return st
}
