// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-21 (Phase 4 Collective Graph). LOCKED ≠ FREEZE (edit dgn izin owner + re-lock).
// 2026-06-22 (owner-approved, audit pre-freeze): (a) edge anti-double-promote pakai LABEL (match caller cognitive_share_job.go — dulu pakai id → ga pernah match → edge re-promote tiap tick); (b) exclude personal diperluas person→(person/persona/trait/preference) biar gate privasi lebih ketat. Re-lock + chattr-freeze.
package agentdb

// federation_cognitive.go — Phase 4 COLLECTIVE GRAPH (D14): promote triple cognitive
// UMUM (general knowledge) dari graph LOKAL agent → router shared brain. File BARU
// (extend, gak nyentuh federation.go drawer yg udah ada). Pola SAMA SelectPromotable.
//
// ⚠️ PRIVASI D8 (LANTAI KERAS, owner): data PERSONAL owner HARAM ke shared/mesh. Filter
// ALLOWLIST (default-DENY): cuma type UMUM yg inherently general boleh promote. Personal
// (person/trait/preference/fact/event/memory/project/persona/doctrine) DITOLAK. Plus wajib
// source_kind='verified' (paling dipercaya) + status='active'. Node yg NYAMBUNG ke identitas
// owner (edge ke/dari node type=person) JUGA ditolak (konteks personal). Filter ke-2 di sisi
// router (safety ganda).

import "fmt"

// promotableCognitiveTypes — ALLOWLIST type yg inherently UMUM (aman shared). Default-deny:
// type di luar ini (person/trait/preference/fact/event/memory/project/persona/doctrine) = TOLAK.
// 'fact' SENGAJA dikecualikan: bisa personal ("Aola lahir 1987") — gak bisa dibedain by-type.
var promotableCognitiveTypes = map[string]bool{
	"concept":   true,
	"skill":     true,
	"knowledge": true,
}

// PromotableCogNode — payload ringkas buat promote (tanpa embedding/personal field).
type PromotableCogNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Why   string `json:"why"`
}

// PromotableCogEdge — edge antar-node yg dua-duanya promotable.
type PromotableCogEdge struct {
	FromLabel    string `json:"from_label"`
	ToLabel      string `json:"to_label"`
	RelationType string `json:"relation_type"`
}

// ensureCognitiveFederationSchema — log anti-double-promote buat cognitive (pisah dari drawer).
func (s *Store) ensureCognitiveFederationSchema() {
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS federation_cognitive_log (
		ref_key     TEXT PRIMARY KEY,   -- "node:<id>" atau "edge:<from>|<rel>|<to>"
		remote_id   TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'ok',
		promoted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
}

// SelectPromotableCognitiveNodes — node UMUM + verified + active + BUKAN personal + BUKAN
// nyambung ke identitas owner (person) + belum di-promote. Default-DENY (privasi D8).
func (s *Store) SelectPromotableCognitiveNodes(limit int) ([]PromotableCogNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	s.ensureCognitiveFederationSchema()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// ALLOWLIST type + verified + active. Exclude node yg punya edge ke/dari node type=person
	// (konteks personal owner). Exclude yg udah ke-promote.
	rows, err := s.db.Query(
		`SELECT n.id, n.label, n.type, n.why
		   FROM cognitive_nodes n
		  WHERE n.status='active'
		    AND n.source_kind='verified'
		    AND n.type IN ('concept','skill','knowledge')
		    AND ('node:'||n.id) NOT IN (SELECT ref_key FROM federation_cognitive_log WHERE status='ok')
		    AND n.id NOT IN (
		         SELECT e.from_id FROM cognitive_edges e JOIN cognitive_nodes p ON e.to_id=p.id   WHERE p.type IN ('person','persona','trait','preference')
		         UNION
		         SELECT e.to_id   FROM cognitive_edges e JOIN cognitive_nodes p ON e.from_id=p.id WHERE p.type IN ('person','persona','trait','preference')
		    )
		  ORDER BY n.hit_count DESC, n.confidence DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("select promotable cognitive nodes: %w", err)
	}
	defer rows.Close()
	out := []PromotableCogNode{}
	for rows.Next() {
		var n PromotableCogNode
		if err := rows.Scan(&n.ID, &n.Label, &n.Type, &n.Why); err != nil {
			return nil, err
		}
		if !promotableCognitiveTypes[n.Type] { // safety ganda (redundant tapi eksplisit)
			continue
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SelectPromotableCognitiveEdges — edge yg KEDUA endpoint-nya node promotable (umum) +
// relation UMUM (bukan personal: prefers/dislikes/values/dll). belum di-promote.
func (s *Store) SelectPromotableCognitiveEdges(limit int) ([]PromotableCogEdge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	s.ensureCognitiveFederationSchema()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Relation UMUM doang (world-fact); relation twin/personal (prefers/dislikes/values/
	// communicates_in_style/thinks_via/decides_by/reacts_when/goal_is) DITOLAK.
	rows, err := s.db.Query(
		`SELECT f.label, t.label, e.relation_type
		   FROM cognitive_edges e
		   JOIN cognitive_nodes f ON e.from_id=f.id
		   JOIN cognitive_nodes t ON e.to_id=t.id
		  WHERE e.status='active'
		    AND f.status='active' AND t.status='active'
		    AND f.source_kind='verified' AND t.source_kind='verified'
		    AND f.type IN ('concept','skill','knowledge')
		    AND t.type IN ('concept','skill','knowledge')
		    AND e.relation_type IN ('is_a','part_of','uses','depends_on','related_to','causes','references','about')
		    AND ('edge:'||f.label||'|'||e.relation_type||'|'||t.label) NOT IN
		        (SELECT ref_key FROM federation_cognitive_log WHERE status='ok')
		  ORDER BY e.strength DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("select promotable cognitive edges: %w", err)
	}
	defer rows.Close()
	out := []PromotableCogEdge{}
	for rows.Next() {
		var e PromotableCogEdge
		if err := rows.Scan(&e.FromLabel, &e.ToLabel, &e.RelationType); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkPromotedCognitive — catat node/edge udah di-promote (anti double). refKey:
// "node:<id>" / "edge:<from>|<rel>|<to>".
func (s *Store) MarkPromotedCognitive(refKey, remoteID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveFederationSchema()
	_, err := s.db.Exec(
		`INSERT INTO federation_cognitive_log (ref_key, remote_id, status, promoted_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(ref_key) DO UPDATE SET remote_id=excluded.remote_id, status=excluded.status, promoted_at=excluded.promoted_at`,
		refKey, remoteID, status)
	return err
}
