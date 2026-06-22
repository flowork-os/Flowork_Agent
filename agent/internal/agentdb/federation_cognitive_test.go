package agentdb

import "testing"

// SelectPromotableCognitiveNodes (infra Phase-4, dulu dorman) — gate privasi D8: cuma
// concept/skill/knowledge + verified + active + BUKAN nyambung identitas owner (person-edge);
// anti-double via federation_cognitive_log.
func TestSelectPromotableCognitiveNodes(t *testing.T) {
	s := openTestStore(t)
	mk := func(id, typ, sk string) {
		if _, err := s.UpsertNode(CogNode{ID: id, Label: id, Type: typ, SourceKind: sk, Status: "active", Confidence: 0.8}); err != nil {
			t.Fatal(err)
		}
	}
	mk("c-ok", "concept", "verified")               // LAYAK
	mk("m-no", "memory", "verified")                // tipe di luar allowlist
	mk("c-unverified", "concept", "agent_inferred") // bukan verified
	mk("c-person", "concept", "verified")           // nyambung ke person → TOLAK
	mk("p1", "person", "verified")
	if err := s.UpsertEdge(CogEdge{FromID: "c-person", ToID: "p1", RelationType: "related_to", Status: "active", Confidence: 0.5, Strength: 1}); err != nil {
		t.Fatal(err)
	}

	got, err := s.SelectPromotableCognitiveNodes(50)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(got) != 1 || got[0].ID != "c-ok" {
		ids := []string{}
		for _, n := range got {
			ids = append(ids, n.ID)
		}
		t.Fatalf("eligible=%v want [c-ok] (allowlist+verified+non-personal)", ids)
	}

	// Anti-double: tandai promoted → ga ke-pilih lagi.
	if err := s.MarkPromotedCognitive("node:c-ok", "remote1", "ok"); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.SelectPromotableCognitiveNodes(50)
	if len(got2) != 0 {
		t.Fatalf("setelah promoted harus 0, dapat %d (anti-double bocor)", len(got2))
	}
}

// QC: edge selector + anti-double LABEL-based (fix 2026-06-22). Dulu key pakai id → ga
// match caller (label) → edge re-promote tiap tick. Sekarang harus dedup bener.
func TestSelectPromotableCognitiveEdges(t *testing.T) {
	s := openTestStore(t)
	mk := func(id string) {
		if _, err := s.UpsertNode(CogNode{ID: id, Label: id, Type: "concept", SourceKind: "verified", Status: "active", Confidence: 0.8}); err != nil {
			t.Fatal(err)
		}
	}
	mk("Go")
	mk("Goroutine")
	if err := s.UpsertEdge(CogEdge{FromID: "Go", ToID: "Goroutine", RelationType: "part_of", Status: "active", SourceKind: "verified", Confidence: 0.7, Strength: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := s.SelectPromotableCognitiveEdges(50)
	if err != nil {
		t.Fatalf("select edges: %v", err)
	}
	if len(got) != 1 || got[0].FromLabel != "Go" || got[0].ToLabel != "Goroutine" {
		t.Fatalf("eligible edges=%v want [Go-part_of->Goroutine]", got)
	}
	// Anti-double pakai refKey LABEL (sama format caller cognitive_share_job.go).
	refKey := "edge:" + got[0].FromLabel + "|" + got[0].RelationType + "|" + got[0].ToLabel
	if err := s.MarkPromotedCognitive(refKey, "r1", "ok"); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.SelectPromotableCognitiveEdges(50)
	if len(got2) != 0 {
		t.Fatalf("setelah promoted harus 0 (anti-double LABEL), dapat %d — fix edge-dedup BOCOR", len(got2))
	}
}
