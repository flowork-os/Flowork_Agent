// cognitive_gate.go — Validation Gate buat CGM (roadmap §4.5, JANTUNG anti-halu).
//
// Sebelum triple jadi 'active': (1) scan antibody (reuse immune.go) → kena pola
// injection/jailbreak → quarantine; (2) confidence < floor (0.3) → quarantine;
// (3) kontradiksi pada relasi FUNGSIONAL (mis. is_a, decides_by) → JANGAN timpa
// diam-diam, catat ke cognitive_tension → owner yang putusin ("tanya besok pagi").
//
// Reuse: matchAntibody + loadAntibodies + quarantineConfidenceFloor (immune.go,
// sepaket). Plug-and-play: file baru, gak modify yang locked.

package agentdb

import (
	"fmt"
	"strings"
)

// FunctionalRelations — relasi yang idealnya 1 target per (from). Target beda =
// indikasi kontradiksi (perlu owner putusin). Relasi lain (related_to/uses/values/…)
// boleh banyak target, gak dianggap kontradiksi.
var FunctionalRelations = map[string]bool{
	"is_a": true, "decides_by": true, "located_in": true, "created_by": true,
	"goal_is": true, "communicates_in_style": true,
}

// GateStatus tentuin status sebuah kandidat (node/edge) dari text + confidence.
// Return ("quarantined", reason) atau ("active", ""). antibodies di-load sekali oleh
// caller (LoadAntibodyPatterns) buat efisiensi batch.
func GateStatus(text string, confidence float64, antibodies []string) (status, reason string) {
	if hit := matchAntibody(text, antibodies); hit != "" {
		return "quarantined", "antibody match: " + hit
	}
	if confidence < quarantineConfidenceFloor {
		return "quarantined", fmt.Sprintf("low confidence %.2f < %.2f", confidence, quarantineConfidenceFloor)
	}
	return "active", ""
}

// LoadAntibodyPatterns expose pattern antibody (seed dulu kalau kosong) buat dipakai
// GateStatus berkali-kali dalam 1 batch digest.
func (s *Store) LoadAntibodyPatterns() ([]string, error) {
	_, _ = s.SeedAntibodies() // idempotent
	return s.loadAntibodies()
}

// DetectEdgeContradiction — buat relasi fungsional, cek ada edge active dengan
// (from_id, relation_type) sama tapi to_id BEDA. Return (oldToID, true) kalau bentrok.
// Relasi non-fungsional → selalu (,"",false) (boleh multi-target).
func (s *Store) DetectEdgeContradiction(fromID, relationType, newToID string) (oldToID string, conflict bool) {
	if !FunctionalRelations[strings.TrimSpace(relationType)] {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()

	err := s.db.QueryRow(
		`SELECT to_id FROM cognitive_edges
		 WHERE from_id=? AND relation_type=? AND status='active' AND to_id<>? LIMIT 1`,
		fromID, relationType, newToID).Scan(&oldToID)
	if err != nil || oldToID == "" {
		return "", false
	}
	return oldToID, true
}

// RecordTension catat kontradiksi (status 'open') buat owner review.
func (s *Store) RecordTension(fromID, relationType, oldToID, newToID, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	_, err := s.db.Exec(
		`INSERT INTO cognitive_tension (from_id, relation_type, old_to_id, new_to_id, detail)
		 VALUES (?,?,?,?,?)`, fromID, relationType, oldToID, newToID, detail)
	if err != nil {
		return fmt.Errorf("record tension: %w", err)
	}
	return nil
}

// CogTension — baris kontradiksi buat GUI/owner.
type CogTension struct {
	ID           int64  `json:"id"`
	FromID       string `json:"from_id"`
	RelationType string `json:"relation_type"`
	OldToID      string `json:"old_to_id"`
	NewToID      string `json:"new_to_id"`
	Detail       string `json:"detail"`
	Status       string `json:"status"`
}

// ListOpenTensions ambil kontradiksi yang belum diputusin owner.
func (s *Store) ListOpenTensions(limit int) ([]CogTension, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()

	rows, err := s.db.Query(
		`SELECT id, from_id, relation_type, old_to_id, new_to_id, detail, status
		 FROM cognitive_tension WHERE status='open' ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CogTension
	for rows.Next() {
		var tn CogTension
		if err := rows.Scan(&tn.ID, &tn.FromID, &tn.RelationType, &tn.OldToID, &tn.NewToID, &tn.Detail, &tn.Status); err != nil {
			return nil, err
		}
		out = append(out, tn)
	}
	return out, rows.Err()
}

// ResolveTension tandai 1 kontradiksi selesai (owner udah putusin).
func (s *Store) ResolveTension(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()
	_, err := s.db.Exec(`UPDATE cognitive_tension SET status='resolved' WHERE id=?`, id)
	return err
}
