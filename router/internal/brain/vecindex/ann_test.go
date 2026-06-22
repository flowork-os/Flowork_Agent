package vecindex

import (
	"math"
	"math/rand"
	"testing"
)

func randUnit(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	var ss float64
	for j := range v {
		v[j] = float32(rng.NormFloat64())
		ss += float64(v[j]) * float64(v[j])
	}
	n := float32(math.Sqrt(ss))
	if n == 0 {
		n = 1
	}
	for j := range v {
		v[j] /= n
	}
	return v
}

func topKSet(hits []Hit) map[string]bool {
	m := map[string]bool{}
	for _, h := range hits {
		m[h.ID] = true
	}
	return m
}

// ANN recall vs flat (ground-truth). Ini gerbang kelayakan: ANN harus mendekati flat.
func TestANN_RecallVsFlat(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const N, dim, centers = 2000, 64, 50
	// Data BER-CLUSTER (mirip embedding asli yg ngumpul per-makna), bukan random murni
	// (random high-dim = worst case, ga ada ANN yg bagus + bukan profil embedding nyata).
	base := make([][]float32, centers)
	for c := 0; c < centers; c++ {
		base[c] = randUnit(rng, dim)
	}
	ids := make([]string, N)
	vecs := make([][]float32, N)
	for i := 0; i < N; i++ {
		ctr := base[i%centers]
		v := make([]float32, dim)
		var ss float64
		for j := range v {
			v[j] = ctr[j] + float32(rng.NormFloat64())*0.15 // noise kecil di sekitar center
			ss += float64(v[j]) * float64(v[j])
		}
		nrm := float32(math.Sqrt(ss))
		for j := range v {
			v[j] /= nrm
		}
		ids[i] = itoa(i)
		vecs[i] = v
	}
	ix, err := Build(ids, vecs)
	if err != nil {
		t.Fatal(err)
	}
	ann := BuildANN(ix, 0) // auto ~sqrt(2000)=44 cluster
	if ann.Len() != N {
		t.Fatalf("ANN.Len=%d want %d", ann.Len(), N)
	}

	const Q, k = 60, 10
	nprobe := ann.nClusters / 3 // ~⅓ cluster → tetap ~3× lebih sedikit scan vs flat
	if nprobe < 1 {
		nprobe = 1
	}
	var totalRecall float64
	for qi := 0; qi < Q; qi++ {
		q := randUnit(rng, dim)
		truth := topKSet(ix.Search(q, k)) // flat = ground truth
		got := ann.Search(q, k, nprobe)
		hit := 0
		for id := range topKSet(got) {
			if truth[id] {
				hit++
			}
		}
		totalRecall += float64(hit) / float64(k)
	}
	recall := totalRecall / Q
	scanFrac := float64(nprobe) / float64(ann.nClusters)
	t.Logf("ANN recall@%d = %.3f (nprobe=%d/%d cluster = %.0f%% scan → ~%.1fx lebih cepet)",
		k, recall, nprobe, ann.nClusters, scanFrac*100, 1/scanFrac)
	if recall < 0.85 {
		t.Fatalf("recall %.3f < 0.85 — ANN ga layak", recall)
	}
}

// nprobe >= nClusters → ANN identik flat (recall 1.0). Bukti benar (exact di semua cluster).
func TestANN_FullProbeEqualsFlat(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const N, dim = 500, 32
	ids := make([]string, N)
	vecs := make([][]float32, N)
	for i := 0; i < N; i++ {
		ids[i] = itoa(i)
		vecs[i] = randUnit(rng, dim)
	}
	ix, _ := Build(ids, vecs)
	ann := BuildANN(ix, 20)
	q := randUnit(rng, dim)
	flat := topKSet(ix.Search(q, 10))
	full := topKSet(ann.Search(q, 10, 20)) // nprobe=nClusters → semua kandidat
	for id := range flat {
		if !full[id] {
			t.Fatalf("full-probe ANN harus == flat, id %s ilang", id)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
