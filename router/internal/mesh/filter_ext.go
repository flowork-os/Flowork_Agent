// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package mesh

import (
	"database/sql"
	"os"
	"strings"
)

type MeshFilter struct {
	Name   string
	Switch string
	Run    func(db *sql.DB, pkt Packet, drawerContent string) FilterDecision
}

var extraMeshFilters []MeshFilter

func RegisterMeshFilter(f MeshFilter) {
	if f.Run == nil {
		return
	}
	extraMeshFilters = append(extraMeshFilters, f)
}

func meshFilterSwitchOn(key string) bool {
	if strings.TrimSpace(key) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

func runExtraMeshFilters(db *sql.DB, pkt Packet, drawerContent string) ([]FilterDecision, bool) {
	var out []FilterDecision
	reject := false
	for _, f := range extraMeshFilters {
		if !meshFilterSwitchOn(f.Switch) {
			continue
		}
		d := safeRunMeshFilter(f, db, pkt, drawerContent)
		out = append(out, d)
		if d.Decision == "reject" {
			reject = true
		}
	}
	return out, reject
}

func safeRunMeshFilter(f MeshFilter, db *sql.DB, pkt Packet, drawerContent string) (d FilterDecision) {
	defer func() {
		if r := recover(); r != nil {
			d = FilterDecision{Layer: "ext-" + f.Name, Decision: "pass", Reason: "filter panic recovered"}
		}
	}()
	return f.Run(db, pkt, drawerContent)
}
