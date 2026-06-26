// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package brain

import (
	"context"
	"database/sql"
)

type GraphProjection struct {
	Name   string
	Switch string
	Run    func(ctx context.Context, tx *sql.Tx) (int, error)
}

var extraGraphProjections []GraphProjection

func RegisterGraphProjection(p GraphProjection) {
	if p.Run == nil {
		return
	}
	extraGraphProjections = append(extraGraphProjections, p)
}

func runExtraGraphProjectionsTx(ctx context.Context, tx *sql.Tx) int {
	total := 0
	for _, p := range extraGraphProjections {
		if p.Switch != "" && !extraSwitchOn(p.Switch) {
			continue
		}
		n, err := p.Run(ctx, tx)
		if err == nil {
			total += n
		}
	}
	return total
}
