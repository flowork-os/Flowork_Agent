// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Anchor tier-2: fallback kalau super_scrit.md DIHAPUS dari PC. integrity.go hash file2 ini,
// banding root ke const → gate jalan TANPA file manifest. File ini tier-1 (chattr+i), TIDAK
// masuk tier-2 (biar root stabil). ⚠️ FROZEN. Dok: lock/integrity.md + mesh-sharing.md
package mesh

var tier2AnchorFiles = []string{
	"../router/internal/mesh/blocklist.go",
	"../router/internal/mesh/consensus_phase3.go",
	"../router/internal/mesh/crdt.go",
	"../router/internal/mesh/crdt_sets.go",
	"../router/internal/mesh/discovery.go",
	"../router/internal/mesh/filter_ext.go",
	"../router/internal/mesh/filter_integrity.go",
	"../router/internal/mesh/gossip.go",
	"../router/internal/mesh/identity.go",
	"../router/internal/mesh/integrity.go",
	"../router/internal/mesh/karma_gate.go",
	"../router/internal/mesh/karma_toolshare_filter.go",
	"../router/internal/mesh/knowledge.go",
	"../router/internal/mesh/lora.go",
	"../router/internal/mesh/packet.go",
	"../router/internal/mesh/peers.go",
	"../router/internal/mesh/pipeline.go",
	"../router/internal/mesh/sign.go",
	"../router/internal/mesh/similarity.go",
	"../router/internal/mesh/toolvalidate.go",
	"../router/internal/ingest/ingest.go",
	"../router/internal/ingest/sanitize.go",
	"../router/internal/ingest/score.go",
	"../router/handlers_mesh.go",
	"../router/handlers_mesh_advanced.go",
	"../router/handlers_mesh_transport.go",
	"../router/handlers_mesh_stack.go",
	"../router/handlers_mesh_ratelimit.go",
	"../router/handlers_brain_ingest.go",
	"../router/internal/mesh/filter_meshpolicy.go",
	"../router/handlers_mesh_approve_ext.go",
	"../router/internal/mesh/policy.go",
}

// di-set setelah integrity.go final (lihat build-step). Kosong = anchor mati (fail-open).
const tier2AnchorRoot = "fdc6720122a435db3517233afd718e1e69e5a71619dd002535bfa42c38e5401b"
