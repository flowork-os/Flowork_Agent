// cognitive_archive.go — D (PHASE 5) SCALE & LONGEVITY: COLD-ARCHIVE node graph yg lama +
// jarang-kepake biar recall tetep cepet pas graph membengkak. FILE BARU (additive, ga nyentuh
// cognitive_graph.go / cognitive_recall.go yg FROZEN).
//
// Cara kerja: node tua + low-hit + tipe-BULK → status='archived'. Recall (SearchNodesByEmbedding
// & SearchNodesByLabel) udah filter status='active' → archived OTOMATIS ke-skip (ga perlu ubah
// recall). REVERSIBLE (restore = balik ke 'active'); BUKAN delete.
//
// ⚠️ GATED (anti-premature): no-op kalau node aktif < ambang (mis. 50k). Sekarang ~2k node =
// ga ngapa-ngapain (0 dampak recall). Aktif sendiri pas graph beneran gede. Tipe identitas/
// governance/skill/instinct TIDAK PERNAH di-archive (default-DENY: cuma archivableTypes).

package agentdb

import (
	"fmt"
	"time"
)

// archivableTypes — cuma tipe BULK/transient yg aman di-archive pas skala besar. Default-DENY:
// identitas/governance/skill/instinct/persona dst SENGAJA di luar → JANGAN PERNAH ke-archive
// (itu inti yg harus selalu ke-recall).
var archivableTypes = map[string]bool{
	"memory": true, "event": true, "fact": true, "knowledge": true,
	"concept": true, "code": true,
}

func archivableTypeList() []any {
	out := make([]any, 0, len(archivableTypes))
	for t := range archivableTypes {
		out = append(out, t)
	}
	return out
}

func placeholders(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += ","
		}
		s += "?"
	}
	return s
}

// CountActiveNodes — jumlah node status='active' (buat gate ambang archive).
func (s *Store) CountActiveNodes() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cognitive_nodes WHERE status='active'`).Scan(&n)
	return n, err
}

// ArchiveColdNodes — archive node 'active' tua (last_seen_at < cutoff) + low-hit (hit_count<=
// maxHit) + tipe BULK → 'archived'. GATE: kalau node aktif <= activateThreshold → no-op (return
// 0). Reversible. Return jumlah ke-archive.
func (s *Store) ArchiveColdNodes(olderThanDays int, maxHit int64, activateThreshold int) (int, error) {
	if olderThanDays <= 0 {
		olderThanDays = 90
	}
	if maxHit <= 0 {
		maxHit = 1
	}
	active, err := s.CountActiveNodes()
	if err != nil {
		return 0, err
	}
	if active <= activateThreshold {
		return 0, nil // belum skala → JANGAN archive (anti-premature)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	cutoff := time.Now().UTC().AddDate(0, 0, -olderThanDays).Format(time.RFC3339)
	types := archivableTypeList()
	args := append([]any{}, types...)
	args = append(args, maxHit, cutoff)
	res, err := s.db.Exec(
		`UPDATE cognitive_nodes SET status='archived'
		  WHERE status='active'
		    AND type IN (`+placeholders(len(types))+`)
		    AND hit_count <= ?
		    AND last_seen_at < ?`, args...)
	if err != nil {
		return 0, fmt.Errorf("archive cold nodes: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RestoreArchivedNode — balikin 1 node dari 'archived' ke 'active' (reversible). Dipakai
// kalau node archived ternyata masih kepake.
func (s *Store) RestoreArchivedNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	_, err := s.db.Exec(`UPDATE cognitive_nodes SET status='active' WHERE id=? AND status='archived'`, id)
	if err != nil {
		return fmt.Errorf("restore archived node: %w", err)
	}
	return nil
}

// CountArchivedNodes — jumlah node status='archived' (observability/restore).
func (s *Store) CountArchivedNodes() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cognitive_nodes WHERE status='archived'`).Scan(&n)
	return n, err
}
