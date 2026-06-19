package agentdb

import (
	"math"
	"testing"
)

func TestQuantizeAndCosine(t *testing.T) {
	a := Quantize([]float32{1, 0, 0})
	b := Quantize([]float32{1, 0, 0})       // identical → cosine ~1
	c := Quantize([]float32{0, 1, 0})       // orthogonal → cosine ~0
	d := Quantize([]float32{0.9, 0.1, 0})   // similar to a

	if got := CosineQ(a, b); math.Abs(got-1.0) > 0.02 {
		t.Fatalf("identical cosine = %v, want ~1", got)
	}
	if got := CosineQ(a, c); math.Abs(got) > 0.02 {
		t.Fatalf("orthogonal cosine = %v, want ~0", got)
	}
	if got := CosineQ(a, d); got < 0.95 {
		t.Fatalf("similar cosine = %v, want >0.95", got)
	}
	if CosineQ(Quantize([]float32{0, 0, 0}), a) != 0 {
		t.Fatal("zero vector should give 0 cosine")
	}
}

func TestResolveByEmbedding_MergeVsNew(t *testing.T) {
	s := openTestStore(t)

	// existing node "kendaraan" with an embedding
	embKendaraan := Quantize([]float32{0.8, 0.2, 0.0})
	if _, err := s.UpsertNode(CogNode{
		ID: "a/concept/kendaraan", Label: "kendaraan", Type: "concept", Embedding: embKendaraan,
	}); err != nil {
		t.Fatal(err)
	}

	// query "mobil" — near-identical meaning → should resolve to existing
	embMobil := Quantize([]float32{0.82, 0.18, 0.0})
	id, score, found := s.ResolveByEmbedding("concept", embMobil, 0)
	if !found || id != "a/concept/kendaraan" {
		t.Fatalf("expected merge to kendaraan (score=%v found=%v id=%q)", score, found, id)
	}

	// query "pisang" — different meaning → no merge
	embPisang := Quantize([]float32{0.0, 0.1, 0.99})
	_, _, found2 := s.ResolveByEmbedding("concept", embPisang, 0)
	if found2 {
		t.Fatal("dissimilar embedding should NOT merge")
	}

	// wrong type → no candidates
	if _, _, f := s.ResolveByEmbedding("person", embMobil, 0); f {
		t.Fatal("type mismatch should not resolve")
	}

	// empty query → false (caller falls back)
	if _, _, f := s.ResolveByEmbedding("concept", nil, 0); f {
		t.Fatal("empty embedding should return false")
	}
}
