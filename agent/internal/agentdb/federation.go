// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-03
// Reason: Roadmap 2 B6 federation. Verified: promote local->router shared (findable
//   by others), quality-gate (quarantine excluded), anti double-promote, resilient
//   (router down graceful). Extend -> file baru, JANGAN modify ini.
//
// federation.go — Roadmap 2 Fase B6: federation lokal ↔ router shared (OPSIONAL).
//
// Promote drawer brain LOKAL yang berharga → korpus SHARED router, biar warga
// lain bisa belajar (pull via brain_search_shared). OPSIONAL + resilient: ini
// nilai tambah, BUKAN syarat — router mati, agent tetep jalan penuh (brain lokal).
//
// Storage-only di sini (no network — orchestrasi promote di tool layer biar
// agentdb ga kopling ke routerclient). Quality-gate: non-quarantine, confidence
// tinggi, mem_type aman (experience/eureka/fact) — JANGAN promote constitution/
// user/secret. Sync log cegah double-promote.

package agentdb

import "fmt"

// minPromoteConfidence — quality-gate: cuma drawer confidence >= ini yg di-share.
const minPromoteConfidence = 0.7

// PromotableDrawer — drawer kandidat promote ke shared.
type PromotableDrawer struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Wing    string `json:"wing"`
	Room    string `json:"room"`
	MemType string `json:"mem_type"`
}

func (s *Store) ensureFederationSchema() {
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS federation_sync_log (
		drawer_id   TEXT PRIMARY KEY,
		remote_id   TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'ok',
		promoted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
}

// SelectPromotable — drawer lokal yang LAYAK di-share ke router shared.
// Gate: live, non-quarantine, confidence>=floor, mem_type aman, belum pernah
// di-promote sukses. EXCLUDE constitution/user (sensitif).
func (s *Store) SelectPromotable(limit int) ([]PromotableDrawer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureBrainSchema()
	s.ensureFederationSchema()
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT d.id, d.content, d.wing, d.room, d.mem_type
		   FROM brain_drawers d
		  WHERE d.deleted_at IS NULL
		    AND d.quarantined = 0
		    AND d.confidence >= ?
		    AND d.mem_type IN ('experience','eureka','fact')
		    AND d.id NOT IN (SELECT drawer_id FROM federation_sync_log WHERE status='ok')
		  ORDER BY d.importance DESC, d.created_at DESC
		  LIMIT ?`, minPromoteConfidence, limit)
	if err != nil {
		return nil, fmt.Errorf("select promotable: %w", err)
	}
	defer rows.Close()
	out := []PromotableDrawer{}
	for rows.Next() {
		var d PromotableDrawer
		if err := rows.Scan(&d.ID, &d.Content, &d.Wing, &d.Room, &d.MemType); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkPromoted catat hasil promote (status 'ok'/'error') biar ga double-promote.
func (s *Store) MarkPromoted(drawerID, remoteID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureFederationSchema()
	if status == "" {
		status = "ok"
	}
	_, err := s.db.Exec(
		`INSERT INTO federation_sync_log (drawer_id, remote_id, status, promoted_at)
		 VALUES (?, ?, ?, datetime('now'))
		 ON CONFLICT(drawer_id) DO UPDATE SET remote_id=excluded.remote_id,
		   status=excluded.status, promoted_at=excluded.promoted_at`,
		drawerID, remoteID, status)
	return err
}

// CountPromoted — jumlah drawer yang udah sukses di-share (buat status/test).
func (s *Store) CountPromoted() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureFederationSchema()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM federation_sync_log WHERE status='ok'`).Scan(&n)
	return n, err
}
