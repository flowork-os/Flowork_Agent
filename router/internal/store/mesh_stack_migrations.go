// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

func init() {
	RegisterMigration(Migration{
		ID:   101,
		Name: "section14_23_mesh_stack",
		SQL: `
-- Section 14: transport + packet + relay
CREATE TABLE IF NOT EXISTS mesh_packets (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  packet_id     TEXT NOT NULL UNIQUE,
  origin_pubkey TEXT NOT NULL,
  packet_type   TEXT NOT NULL,
  payload_json  TEXT NOT NULL DEFAULT '{}',
  signature     TEXT NOT NULL DEFAULT '',
  ttl           INTEGER NOT NULL DEFAULT 5,
  hop_count     INTEGER NOT NULL DEFAULT 0,
  received_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  processed     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_mesh_packets_type ON mesh_packets(packet_type);
CREATE INDEX IF NOT EXISTS idx_mesh_packets_processed ON mesh_packets(processed);

-- Section 15: gossip state
CREATE TABLE IF NOT EXISTS mesh_gossip_state (
  packet_id   TEXT PRIMARY KEY,
  seen_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  forwarded_to TEXT NOT NULL DEFAULT ''
);

-- Section 16: CRDT vector clocks
CREATE TABLE IF NOT EXISTS mesh_crdt_state (
  topic       TEXT NOT NULL,
  node_pubkey TEXT NOT NULL,
  counter     INTEGER NOT NULL DEFAULT 0,
  payload_json TEXT NOT NULL DEFAULT '{}',
  updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (topic, node_pubkey)
);

-- Section 17: knowledge share
CREATE TABLE IF NOT EXISTS mesh_knowledge_inbox (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  packet_id    TEXT NOT NULL UNIQUE,
  origin_pubkey TEXT NOT NULL,
  drawer_content TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'shadow',
  arrived_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mesh_knowledge_status ON mesh_knowledge_inbox(status);

-- Section 18: tool manifest sharing
CREATE TABLE IF NOT EXISTS mesh_tool_manifests (
  tool_name    TEXT NOT NULL,
  origin_pubkey TEXT NOT NULL,
  manifest_json TEXT NOT NULL DEFAULT '{}',
  signature    TEXT NOT NULL DEFAULT '',
  arrived_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tool_name, origin_pubkey)
);

-- Section 19: karma per-peer
CREATE TABLE IF NOT EXISTS mesh_peer_karma (
  pubkey_hex  TEXT PRIMARY KEY,
  karma       REAL NOT NULL DEFAULT 0.5,
  packets_promoted INTEGER NOT NULL DEFAULT 0,
  packets_dropped  INTEGER NOT NULL DEFAULT 0,
  last_event_at    TEXT
);

-- Section 20: filter pipeline audit
CREATE TABLE IF NOT EXISTS mesh_filter_audit (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  packet_id   TEXT NOT NULL,
  filter_name TEXT NOT NULL,
  decision    TEXT NOT NULL,
  reason      TEXT NOT NULL DEFAULT '',
  occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mesh_filter_packet ON mesh_filter_audit(packet_id);

-- Section 21: LoRA delta sync
CREATE TABLE IF NOT EXISTS mesh_lora_deltas (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  model_name   TEXT NOT NULL,
  origin_pubkey TEXT NOT NULL,
  delta_uri    TEXT NOT NULL,
  delta_size   INTEGER NOT NULL DEFAULT 0,
  signature    TEXT NOT NULL DEFAULT '',
  received_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Section 22: L3 semantic sync fallback
CREATE TABLE IF NOT EXISTS mesh_l3_state (
  k           TEXT PRIMARY KEY,
  v           TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) WITHOUT ROWID;

-- Section 23: mesh daemon state (heartbeat tracking)
CREATE TABLE IF NOT EXISTS mesh_daemon_status (
  k           TEXT PRIMARY KEY,
  v           TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) WITHOUT ROWID;
`,
	})
}
