// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 16 phase 2 CRDT merge logic. G-Counter via counter
//   field, LWW via timestamp comparison. Phase 3 (G-Set/2P-Set state-
//   based merge, vector clock proper) → tambah file baru.
//
// crdt.go — Section 16 phase 2: CRDT state merge for mesh.

package mesh

import (
	"database/sql"
	"time"
)

// CRDTUpsert — G-Counter style. Increment counter for (topic, node_pubkey).
// Caller passes payload_json which is replaced atomically.
func CRDTUpsert(db *sql.DB, topic, nodePubkey string, counter int64, payloadJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO mesh_crdt_state (topic, node_pubkey, counter, payload_json, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(topic, node_pubkey) DO UPDATE SET
		   counter = CASE WHEN excluded.counter > mesh_crdt_state.counter
		                  THEN excluded.counter ELSE mesh_crdt_state.counter END,
		   payload_json = CASE WHEN excluded.updated_at > mesh_crdt_state.updated_at
		                       THEN excluded.payload_json ELSE mesh_crdt_state.payload_json END,
		   updated_at = CASE WHEN excluded.updated_at > mesh_crdt_state.updated_at
		                     THEN excluded.updated_at ELSE mesh_crdt_state.updated_at END`,
		topic, nodePubkey, counter, payloadJSON, now,
	)
	return err
}

// CRDTAggregate — sum counter across all peers for a topic. G-Counter
// total = SUM(per-peer counter).
func CRDTAggregate(db *sql.DB, topic string) (int64, error) {
	var sum int64
	err := db.QueryRow(
		`SELECT COALESCE(SUM(counter), 0) FROM mesh_crdt_state WHERE topic = ?`,
		topic).Scan(&sum)
	return sum, err
}

// CRDTListByTopic — return all node entries for a topic.
type CRDTEntry struct {
	NodePubkey  string `json:"node_pubkey"`
	Counter     int64  `json:"counter"`
	PayloadJSON string `json:"payload_json"`
	UpdatedAt   string `json:"updated_at"`
}

func CRDTListByTopic(db *sql.DB, topic string) ([]CRDTEntry, error) {
	rows, err := db.Query(
		`SELECT node_pubkey, counter, payload_json, updated_at
		 FROM mesh_crdt_state WHERE topic = ?
		 ORDER BY updated_at DESC`, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CRDTEntry{}
	for rows.Next() {
		var e CRDTEntry
		if serr := rows.Scan(&e.NodePubkey, &e.Counter, &e.PayloadJSON, &e.UpdatedAt); serr != nil {
			return nil, serr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
