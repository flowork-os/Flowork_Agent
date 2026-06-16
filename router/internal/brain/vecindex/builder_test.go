package vecindex

import (
	"strconv"
	"testing"
)

// TestBuilderMatchesBuild — Builder streaming dgn scale yang sama = hasil identik dengan Build.
func TestBuilderMatchesBuild(t *testing.T) {
	dim := 48
	var ids []string
	var vecs [][]float32
	for i := 0; i < 100; i++ {
		ids = append(ids, strconv.Itoa(i))
		vecs = append(vecs, unit(dim, i+500))
	}
	full, err := Build(ids, vecs)
	if err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(dim, full.scale) // pakai scale yg sama yg Build hitung
	for i := range ids {
		if err := b.Add(ids[i], vecs[i]); err != nil {
			t.Fatal(err)
		}
	}
	stream := b.Finish()
	if stream.Len() != full.Len() {
		t.Fatalf("len %d != %d", stream.Len(), full.Len())
	}
	for _, qi := range []int{0, 33, 99} {
		fa := full.Search(vecs[qi], 5)
		sa := stream.Search(vecs[qi], 5)
		if len(fa) != len(sa) {
			t.Fatalf("q%d hit count differ", qi)
		}
		for i := range fa {
			if fa[i].ID != sa[i].ID || fa[i].Score != sa[i].Score {
				t.Errorf("q%d pos%d: %+v != %+v", qi, i, fa[i], sa[i])
			}
		}
	}
}
