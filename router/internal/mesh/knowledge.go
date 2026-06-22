// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 17 phase 2 knowledge share pipeline. Inbox shadow →
//   quarantine → promoted (or dropped). Phase 3 (cosine validate vs
//   existing brain, consensus N-of-M peer endorsement) → tambah file
//   baru.
//
// knowledge.go — Section 17 phase 2: knowledge inbox state machine.

package mesh

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	StatusShadow      = "shadow"
	StatusQuarantine  = "quarantine"
	StatusPromoted    = "promoted"
	StatusInvalidated = "invalidated"
	StatusDropped     = "dropped"
)

// IngestKnowledge — peer kirim drawer content. INSERT atau OR IGNORE.
func IngestKnowledge(db *sql.DB, packetID, originPubkey, drawerContent string) error {
	if strings.TrimSpace(packetID) == "" {
		return fmt.Errorf("packet_id required")
	}
	if strings.TrimSpace(drawerContent) == "" {
		return fmt.Errorf("drawer_content required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT OR IGNORE INTO mesh_knowledge_inbox
		   (packet_id, origin_pubkey, drawer_content, status, arrived_at)
		 VALUES (?, ?, ?, ?, ?)`,
		packetID, originPubkey, drawerContent, StatusShadow, now,
	)
	return err
}

// PromoteKnowledge — caller (filter pipeline) advance status. Valid
// transitions: shadow → quarantine → promoted, atau langsung dropped.
func PromoteKnowledge(db *sql.DB, packetID, newStatus string) error {
	valid := map[string]bool{
		StatusQuarantine: true, StatusPromoted: true,
		StatusInvalidated: true, StatusDropped: true,
	}
	if !valid[newStatus] {
		return fmt.Errorf("invalid status %q", newStatus)
	}
	res, err := db.Exec(
		`UPDATE mesh_knowledge_inbox SET status = ? WHERE packet_id = ?`,
		newStatus, packetID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("packet_id %q not found", packetID)
	}
	return nil
}

// KnowledgeEntry mirror row.
type KnowledgeEntry struct {
	ID            int64  `json:"id"`
	PacketID      string `json:"packet_id"`
	OriginPubkey  string `json:"origin_pubkey"`
	DrawerContent string `json:"drawer_content"`
	Status        string `json:"status"`
	ArrivedAt     string `json:"arrived_at"`
}

// ListKnowledge — paginated by status.
func ListKnowledge(db *sql.DB, status string, limit int) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT id, packet_id, origin_pubkey, drawer_content, status, arrived_at
	      FROM mesh_knowledge_inbox WHERE 1=1`
	args := []any{}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeEntry{}
	for rows.Next() {
		var e KnowledgeEntry
		if serr := rows.Scan(&e.ID, &e.PacketID, &e.OriginPubkey,
			&e.DrawerContent, &e.Status, &e.ArrivedAt); serr != nil {
			return nil, serr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountKnowledge per status.
func CountKnowledge(db *sql.DB) (map[string]int, error) {
	out := map[string]int{}
	rows, err := db.Query(
		`SELECT status, COUNT(*) FROM mesh_knowledge_inbox GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var n int
		_ = rows.Scan(&s, &n)
		out[s] = n
	}
	return out, rows.Err()
}
