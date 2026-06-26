// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

func init() {
	RegisterMigration(Migration{
		ID:   100,
		Name: "section13_mesh_foundation",
		SQL: `
CREATE TABLE IF NOT EXISTS mesh_identity (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL DEFAULT ''
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS mesh_peers (
  pubkey_hex     TEXT PRIMARY KEY,
  hostname       TEXT NOT NULL DEFAULT '',
  ip             TEXT NOT NULL DEFAULT '',
  port           INTEGER NOT NULL DEFAULT 0,
  version        TEXT NOT NULL DEFAULT '',
  is_virt        INTEGER NOT NULL DEFAULT 0,
  first_seen_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  trust_score    REAL NOT NULL DEFAULT 0.5,
  blocked        INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_mesh_peers_lastseen ON mesh_peers(last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_mesh_peers_blocked ON mesh_peers(blocked);
`,
	})
}
