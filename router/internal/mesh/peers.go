// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 13 phase 1 (peer registry CRUD). API stable: Upsert /
//   List / SetBlocked / Touch. Phase 2 (trust score recalc, eviction,
//   stale GC, gossip integration) → tambah file baru, JANGAN modify.
//
// peers.go — mesh_peers CRUD. Phase 1: pure SQL — ngga ada actual
// network discovery (single-owner). Endpoint admin bisa Upsert peer
// manual, dan future phase 2 mDNS goroutine bakal Upsert automatic.

package mesh

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Peer mirrors `mesh_peers` row.
type Peer struct {
	PubKeyHex   string  `json:"pubkey_hex"`
	Hostname    string  `json:"hostname"`
	IP          string  `json:"ip"`
	Port        int     `json:"port"`
	Version     string  `json:"version"`
	IsVirt      bool    `json:"is_virt"`
	FirstSeenAt string  `json:"first_seen_at"`
	LastSeenAt  string  `json:"last_seen_at"`
	TrustScore  float64 `json:"trust_score"`
	Blocked     bool    `json:"blocked"`
}

// UpsertPeer insert atau update by pubkey. Set last_seen_at = now.
// Reject kalau IP ada di cloud metadata blocklist.
func UpsertPeer(db *sql.DB, p Peer) error {
	if db == nil {
		return fmt.Errorf("mesh: nil db")
	}
	if strings.TrimSpace(p.PubKeyHex) == "" {
		return fmt.Errorf("mesh: empty pubkey_hex")
	}
	if p.IP != "" && IsCloudMetadataIP(p.IP) {
		return fmt.Errorf("mesh: blocked cloud metadata IP %q", p.IP)
	}
	if p.Hostname != "" && IsCloudMetadataHost(p.Hostname) {
		return fmt.Errorf("mesh: blocked cloud metadata host %q", p.Hostname)
	}
	if p.TrustScore <= 0 {
		p.TrustScore = 0.5
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO mesh_peers
		   (pubkey_hex, hostname, ip, port, version, is_virt,
		    first_seen_at, last_seen_at, trust_score, blocked)
		 VALUES (?,?,?,?,?,?,?,?,?,0)
		 ON CONFLICT(pubkey_hex) DO UPDATE SET
		   hostname     = excluded.hostname,
		   ip           = excluded.ip,
		   port         = excluded.port,
		   version      = excluded.version,
		   is_virt      = excluded.is_virt,
		   last_seen_at = excluded.last_seen_at`,
		p.PubKeyHex, p.Hostname, p.IP, p.Port, p.Version, boolToInt(p.IsVirt),
		now, now, p.TrustScore,
	)
	return err
}

// ListPeers returns all peers ordered by last_seen DESC. Filter blocked
// kalau includeBlocked=false.
func ListPeers(db *sql.DB, includeBlocked bool) ([]Peer, error) {
	if db == nil {
		return nil, fmt.Errorf("mesh: nil db")
	}
	q := `SELECT pubkey_hex, hostname, ip, port, version, is_virt,
	             first_seen_at, last_seen_at, trust_score, blocked
	      FROM mesh_peers`
	if !includeBlocked {
		q += ` WHERE blocked = 0`
	}
	q += ` ORDER BY last_seen_at DESC`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Peer{}
	for rows.Next() {
		var p Peer
		var isVirt, blocked int
		if err := rows.Scan(
			&p.PubKeyHex, &p.Hostname, &p.IP, &p.Port, &p.Version,
			&isVirt, &p.FirstSeenAt, &p.LastSeenAt, &p.TrustScore, &blocked,
		); err != nil {
			return nil, err
		}
		p.IsVirt = isVirt != 0
		p.Blocked = blocked != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetBlocked toggle blocked flag. Block manual via admin endpoint.
func SetBlocked(db *sql.DB, pubKeyHex string, blocked bool) error {
	if db == nil {
		return fmt.Errorf("mesh: nil db")
	}
	res, err := db.Exec(
		`UPDATE mesh_peers SET blocked = ? WHERE pubkey_hex = ?`,
		boolToInt(blocked), pubKeyHex,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("mesh: peer %q not found", pubKeyHex)
	}
	return nil
}

// Touch update last_seen_at = now. Future phase 2 heartbeat loop pakai.
func Touch(db *sql.DB, pubKeyHex string) error {
	if db == nil {
		return fmt.Errorf("mesh: nil db")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`UPDATE mesh_peers SET last_seen_at = ? WHERE pubkey_hex = ?`,
		now, pubKeyHex,
	)
	return err
}

// CountPeers return total peer count (untuk /api/mesh/identity meta).
func CountPeers(db *sql.DB, includeBlocked bool) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("mesh: nil db")
	}
	q := `SELECT COUNT(*) FROM mesh_peers`
	if !includeBlocked {
		q += ` WHERE blocked = 0`
	}
	var n int
	err := db.QueryRow(q).Scan(&n)
	return n, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
