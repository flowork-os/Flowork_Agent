// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-22 (F2 consensus N-of-M). Edit + re-lock.
//
// consensus_phase3.go — Section 20 Phase 3: L8 consensus N-of-M (peer endorsement).
//
// AKAR (roadmap F2): sebelum knowledge dari MESH peer di-promote ke brain, butuh
// ENDORSEMENT: ≥N peer DISTINCT yg ngirim konten near-same (mereka "vouch" knowledge
// itu) — biar 1 peer jahat sendirian GA bisa nyuntik brain kolektif. Plus trusted
// fast-path: 1 peer ber-karma tinggi (track-record bagus) boleh promote tanpa quorum
// penuh (biar mesh sparse ga macet). Kurang dari itu → FLAG → quarantine (tahan sampe
// cukup endorsement / review owner).
//
// ⚠️ Cuma jalur MESH (peer→ProcessKnowledgePacket). Federation OWNER sendiri (INC-4/C
// lewat /api/brain/drawer) TIDAK lewat sini. Single-node (0 peer) = DORMANT (ga ada
// paket peer) → 0 dampak operasi sekarang; aktif pas mesh multi-node hidup.

package mesh

import (
	"database/sql"
	"fmt"
)

const (
	// consensusN — jumlah peer DISTINCT yg harus endorse (kirim near-same) sebelum promote.
	consensusN = 2
	// trustedKarmaFastPath — 1 peer dgn karma >= ini boleh promote tanpa quorum penuh
	// (track-record terpercaya). 0.8 = jauh di atas baseline 0.5, butuh banyak promote sukses.
	trustedKarmaFastPath = 0.8
)

// consensusL8 — L8 gate. pass kalau quorum tercapai ATAU peer trusted; flag (tahan) kalau belum.
func consensusL8(db *sql.DB, pkt Packet, content string) FilterDecision {
	endorsers := countEndorsers(db, content)
	if endorsers >= consensusN {
		return FilterDecision{Layer: "L8-consensus", Decision: "pass",
			Reason: fmt.Sprintf("consensus %d/%d peer", endorsers, consensusN)}
	}
	if k, _ := GetKarma(db, pkt.OriginPubkey); k >= trustedKarmaFastPath {
		return FilterDecision{Layer: "L8-consensus", Decision: "pass",
			Reason: fmt.Sprintf("trusted peer fast-path (karma %.2f)", k)}
	}
	return FilterDecision{Layer: "L8-consensus", Decision: "flag",
		Reason: fmt.Sprintf("nunggu consensus (%d/%d endorser)", endorsers, consensusN)}
}

// countEndorsers — jumlah peer DISTINCT (origin_pubkey) di inbox yg konten-nya near-same
// (Similarity>=threshold) sama `content`, termasuk paket sekarang (udah di-Ingest). DISTINCT
// → 1 peer spam konten sama ga nginflate (anti-sybil-sederhana; sybil penuh = lapis mesh-id).
func countEndorsers(db *sql.DB, content string) int {
	rows, err := db.Query(
		`SELECT origin_pubkey, drawer_content FROM mesh_knowledge_inbox
		 ORDER BY id DESC LIMIT 500`)
	if err != nil {
		return 0
	}
	defer rows.Close()
	peers := map[string]struct{}{}
	for rows.Next() {
		var pub, existing string
		if rows.Scan(&pub, &existing) != nil {
			continue
		}
		if Similarity(content, existing) >= SimilarityThreshold {
			peers[pub] = struct{}{}
		}
	}
	return len(peers)
}
