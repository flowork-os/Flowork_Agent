// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// crdt_sets.go — Section 16 phase 3: proper state-based CRDTs.
//
// Phase 2 gave us a G-Counter + LWW-Register (crdt.go). This adds the two set
// CRDTs and a vector clock the roadmap listed as deferred:
//
//	VectorClock — per-node logical clock; merge = element-wise max; Compare gives
//	              causal ordering (before / after / concurrent).
//	GSet        — grow-only set; merge = union. Monotonic, always convergent.
//	TwoPhaseSet — add-set + tombstone remove-set; an element is present iff added
//	              and not removed. Remove wins (standard 2P-Set semantics).
//
// All three are pure, deterministic, and commutative/associative/idempotent so
// any merge order converges — the core CRDT guarantee. DB persistence reuses the
// mesh_crdt_state table (payload_json holds the serialized state per node).

package mesh

import (
	"database/sql"
	"encoding/json"
	"sort"
)

// ─── Vector clock ────────────────────────────────────────────────────────────

// VectorClock maps node pubkey → logical counter.
type VectorClock map[string]uint64

// Tick increments this node's own counter (call before emitting an event).
func (vc VectorClock) Tick(self string) { vc[self]++ }

// Merge takes the element-wise maximum of two clocks into the receiver.
func (vc VectorClock) Merge(other VectorClock) {
	for node, c := range other {
		if c > vc[node] {
			vc[node] = c
		}
	}
}

// Ordering result for Compare.
const (
	ClockEqual      = 0
	ClockBefore     = -1
	ClockAfter      = 1
	ClockConcurrent = 2
)

// Compare returns the causal relationship of vc relative to other.
func (vc VectorClock) Compare(other VectorClock) int {
	less, greater := false, false
	seen := map[string]struct{}{}
	for node, c := range vc {
		seen[node] = struct{}{}
		o := other[node]
		if c < o {
			less = true
		} else if c > o {
			greater = true
		}
	}
	for node, o := range other {
		if _, ok := seen[node]; ok {
			continue
		}
		if vc[node] < o { // vc[node] is 0 here
			less = true
		}
	}
	switch {
	case less && greater:
		return ClockConcurrent
	case less:
		return ClockBefore
	case greater:
		return ClockAfter
	default:
		return ClockEqual
	}
}

// ─── G-Set (grow-only) ───────────────────────────────────────────────────────

// GSet is a grow-only set of strings.
type GSet map[string]struct{}

func NewGSet() GSet            { return GSet{} }
func (s GSet) Add(el string)   { s[el] = struct{}{} }
func (s GSet) Has(el string) bool { _, ok := s[el]; return ok }

// Merge unions other into the receiver.
func (s GSet) Merge(other GSet) {
	for el := range other {
		s[el] = struct{}{}
	}
}

// Elements returns the sorted members (deterministic output).
func (s GSet) Elements() []string {
	out := make([]string, 0, len(s))
	for el := range s {
		out = append(out, el)
	}
	sort.Strings(out)
	return out
}

// ─── 2P-Set (two-phase, add + tombstone) ─────────────────────────────────────

// TwoPhaseSet supports add and remove; a removed element can never be re-added
// (standard 2P-Set). Both halves are grow-only G-Sets, so merge is just two
// unions and convergence is guaranteed.
type TwoPhaseSet struct {
	Adds    GSet `json:"adds"`
	Removes GSet `json:"removes"`
}

func NewTwoPhaseSet() *TwoPhaseSet {
	return &TwoPhaseSet{Adds: NewGSet(), Removes: NewGSet()}
}

func (s *TwoPhaseSet) Add(el string)    { s.Adds.Add(el) }
func (s *TwoPhaseSet) Remove(el string) { s.Removes.Add(el) }

// Has reports presence: added AND not tombstoned.
func (s *TwoPhaseSet) Has(el string) bool {
	return s.Adds.Has(el) && !s.Removes.Has(el)
}

// Merge unions both halves of other into the receiver.
func (s *TwoPhaseSet) Merge(other *TwoPhaseSet) {
	s.Adds.Merge(other.Adds)
	s.Removes.Merge(other.Removes)
}

// Elements returns the sorted live members.
func (s *TwoPhaseSet) Elements() []string {
	out := []string{}
	for el := range s.Adds {
		if !s.Removes.Has(el) {
			out = append(out, el)
		}
	}
	sort.Strings(out)
	return out
}

// ─── DB-backed convergent merge over mesh_crdt_state ─────────────────────────

// MergeGSetTopic loads every node's G-Set payload for a topic, unions them with
// the local additions, writes the merged state back under self, and returns the
// converged element list. This is the anti-entropy step: after peers gossip
// their CRDT rows, calling this makes the local view consistent.
func MergeGSetTopic(db *sql.DB, topic, self string, localAdds []string) ([]string, error) {
	merged := NewGSet()
	for _, el := range localAdds {
		merged.Add(el)
	}
	rows, err := db.Query(
		`SELECT payload_json FROM mesh_crdt_state WHERE topic = ?`, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if rows.Scan(&payload) != nil {
			continue
		}
		var els []string
		if json.Unmarshal([]byte(payload), &els) == nil {
			for _, el := range els {
				merged.Add(el)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := merged.Elements()
	blob, _ := json.Marshal(out)
	// Persist the converged set under our own node id (counter = cardinality).
	if uerr := CRDTUpsert(db, topic, self, int64(len(out)), string(blob)); uerr != nil {
		return out, uerr
	}
	return out, nil
}
