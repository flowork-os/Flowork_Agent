// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// karma_gate.go — Section 19 phase 3: karma-driven trust gates.
//
// Closes the audit gap "karma threshold gates (peer karma < 0.2 = blocked
// auto)". Two enforcement points:
//
//	1. Discovery whitelist — a low-karma peer's mDNS announce is ignored, so it
//	   never re-enters mesh_peers after misbehaving.
//	2. AutoBlockLowKarma — a periodic sweep that flips blocked=1 on any peer
//	   whose karma fell below the floor, so gossip stops pushing to it.

package mesh

import "database/sql"

// KarmaFloor — peers at/below this trust score are gated out. Matches the
// L3-karma reject threshold in RunFilterPipeline so the two agree.
const KarmaFloor = 0.2

// KarmaGate returns a discovery WhitelistCheck callback that admits a peer only
// when its karma is above the floor. Unknown peers (no karma row) default to
// 0.5 via GetKarma, so first contact is allowed — trust is earned/lost after.
func KarmaGate(db *sql.DB) func(pubkeyHex string) bool {
	return func(pubkeyHex string) bool {
		k, err := GetKarma(db, pubkeyHex)
		if err != nil {
			return true // fail-open on DB error — never lock out the whole mesh
		}
		return k > KarmaFloor
	}
}

// AutoBlockLowKarma flips blocked=1 for every peer whose karma dropped to/under
// the floor, and unblocks peers that have recovered above it (karma decay or
// later good behaviour). Returns (blocked, unblocked) counts. Safe to call on a
// timer; it's a pair of idempotent UPDATEs.
func AutoBlockLowKarma(db *sql.DB) (int, int) {
	blockRes, err := db.Exec(
		`UPDATE mesh_peers SET blocked = 1
		 WHERE blocked = 0 AND pubkey_hex IN (
		   SELECT pubkey_hex FROM mesh_peer_karma WHERE karma <= ?)`, KarmaFloor)
	var blocked int64
	if err == nil {
		blocked, _ = blockRes.RowsAffected()
	}
	unblockRes, err := db.Exec(
		`UPDATE mesh_peers SET blocked = 0
		 WHERE blocked = 1 AND pubkey_hex IN (
		   SELECT pubkey_hex FROM mesh_peer_karma WHERE karma > ?)`, KarmaFloor)
	var unblocked int64
	if err == nil {
		unblocked, _ = unblockRes.RowsAffected()
	}
	return int(blocked), int(unblocked)
}
