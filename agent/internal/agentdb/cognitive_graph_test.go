package agentdb

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCognitiveGraph_UpsertNodeAndReinforce(t *testing.T) {
	s := openTestStore(t)

	n := CogNode{ID: "agent:mr-flow/twin/aola", Label: "Aola Sahidin", Type: "person", SourceKind: "verified", Confidence: 0.9}
	added, err := s.UpsertNode(n)
	if err != nil || !added {
		t.Fatalf("first upsert: added=%v err=%v (want added=true)", added, err)
	}

	// Re-observe same id → reinforce (added=false, hit_count++).
	added, err = s.UpsertNode(n)
	if err != nil || added {
		t.Fatalf("second upsert: added=%v err=%v (want added=false)", added, err)
	}
	got, ok, err := s.GetNode(n.ID)
	if err != nil || !ok {
		t.Fatalf("GetNode: ok=%v err=%v", ok, err)
	}
	if got.HitCount != 2 {
		t.Fatalf("hit_count = %d, want 2 (reinforce)", got.HitCount)
	}
	if got.Label != "Aola Sahidin" || got.Type != "person" {
		t.Fatalf("node fields wrong: %+v", got)
	}
}

func TestCognitiveGraph_EdgeValidationAndStubNodes(t *testing.T) {
	s := openTestStore(t)

	// valid relation + auto-create stub nodes (FK must not fail)
	if err := s.UpsertEdge(CogEdge{FromID: "a/memory/scam", ToID: "a/instinct/verify", RelationType: "taught"}); err != nil {
		t.Fatalf("valid edge: %v", err)
	}
	// invalid relation → rejected
	if err := s.UpsertEdge(CogEdge{FromID: "x", ToID: "y", RelationType: "ngarang_relasi"}); err == nil {
		t.Fatal("invalid relation_type should be rejected")
	}

	nodes, edges := s.CountCognitiveGraph()
	if nodes != 2 || edges != 1 {
		t.Fatalf("count = (%d nodes, %d edges), want (2,1)", nodes, edges)
	}

	// reinforce edge strength on re-observe
	if err := s.UpsertEdge(CogEdge{FromID: "a/memory/scam", ToID: "a/instinct/verify", RelationType: "taught"}); err != nil {
		t.Fatalf("re-upsert edge: %v", err)
	}
	out, _, err := s.Neighbors("a/memory/scam")
	if err != nil || len(out) != 1 {
		t.Fatalf("Neighbors out: len=%d err=%v (want 1)", len(out), err)
	}
	if out[0].Strength < 2.0 {
		t.Fatalf("edge strength = %v, want >= 2.0 after reinforce", out[0].Strength)
	}
}

func TestCognitiveGraph_GetMissingNode(t *testing.T) {
	s := openTestStore(t)
	_, ok, err := s.GetNode("does/not/exist")
	if err != nil || ok {
		t.Fatalf("missing node: ok=%v err=%v (want ok=false, err=nil)", ok, err)
	}
}
