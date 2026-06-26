// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Integritas frozen-core (anti-tamper mesh) → dok lock/integrity.md  ⚠️ FROZEN — jangan edit.
// Gate via seam RegisterMeshFilter; switch FLOWORK_INTEGRITY_GATE. Pola: lock/frozen-core.md
//

package mesh

import "database/sql"

func init() {
	RegisterMeshFilter(MeshFilter{
		Name:   "core-integrity",
		Switch: "FLOWORK_INTEGRITY_GATE",
		Run: func(db *sql.DB, pkt Packet, drawerContent string) FilterDecision {
			if CoreClean() {
				return FilterDecision{Layer: "L0-core-integrity", Decision: "pass"}
			}
			return FilterDecision{
				Layer:    "L0-core-integrity",
				Decision: "reject",
				Reason:   "frozen-core node ini tampered (root-hash mismatch) — nolak pembelajaran mesh",
			}
		},
	})
}
