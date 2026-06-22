// federation_recovery.go — D32-INC4: pilih recovery-instinct GENERIK (hasil INC-3) buat
// di-share ke shared-brain router → imunitas kolektif (agent lain recall via
// brain_search_shared). FILE BARU (extend, ga nyentuh federation_cognitive.go yg soft-
// locked); REUSE federation_cognitive_log (anti-double-promote) + PromotableCogNode.
//
// ⚠️ PRIVASI D8: cuma instinct where_domain='recovery' + source_kind='verified' + active.
// Instinct ini SUDAH privacy-safe by-construction (INC-3: kunci-kelas + Lapis A strip →
// 0 owner-data). Double-check DETERMINISTIK di sisi promote (host): StripDeterministic
// (label)==label && !ContainsBrand(label) — defense in depth sebelum keluar agent.

package agentdb

import "fmt"

// OwnerNameAllowlist — accessor publik buat host (C/INC-4 double-check privasi): nama personal
// owner (graph type=person) yg WAJIB di-redaksi sebelum konten keluar agent. Best-effort.
func (s *Store) OwnerNameAllowlist() []string { return s.ownerNameAllowlist() }

// SelectPromotableRecoveryInstincts — recovery-instinct AKTIF + verified + belum di-share.
// Default-DENY by-konstruksi (cuma where_domain='recovery' type='instinct'). Anti-double
// lewat federation_cognitive_log (ref_key "node:<id>", status='ok'). Urut hit_count (paling
// terbukti dulu) → confidence.
func (s *Store) SelectPromotableRecoveryInstincts(limit int) ([]PromotableCogNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	s.ensureCognitiveFederationSchema()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT n.id, n.label, n.type, n.why
		   FROM cognitive_nodes n
		  WHERE n.status='active'
		    AND n.type='instinct'
		    AND n.where_domain='recovery'
		    AND n.source_kind='verified'
		    AND ('node:'||n.id) NOT IN (SELECT ref_key FROM federation_cognitive_log WHERE status='ok')
		  ORDER BY n.hit_count DESC, n.confidence DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("select promotable recovery instincts: %w", err)
	}
	defer rows.Close()
	out := []PromotableCogNode{}
	for rows.Next() {
		var n PromotableCogNode
		if err := rows.Scan(&n.ID, &n.Label, &n.Type, &n.Why); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
