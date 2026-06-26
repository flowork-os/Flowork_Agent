// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

func init() {
	RegisterMigration(Migration{
		ID:   99002,
		Name: "mesh_packets_timestamp_ns",
		SQL:  `ALTER TABLE mesh_packets ADD COLUMN timestamp_ns INTEGER NOT NULL DEFAULT 0;`,
	})
}
