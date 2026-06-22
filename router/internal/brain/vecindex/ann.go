// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-22 (ANN IVF, recall@10=0.918). Edit + re-lock.
//
// ann.go — IVF ANN (approximate nearest-neighbor) buat skala BESAR (>jutaan vektor).
//
// AKAR (roadmap ANN): Search flat = O(N) scan tiap query. Di jutaan vektor lambat. IVF:
// partisi vektor ke CLUSTER (k-means) → query probe `nprobe` cluster terdekat → SearchSubset
// (EXACT di kandidat itu). O(nClusters + kandidat) << O(N). Recall tunable via nprobe
// (nprobe>=nClusters = identik flat).
//
// ⚠️ ADDITIVE & SAFE (owner: "jangan rip-replace index 860k yg jalan"): Index flat TIDAK
// disentuh — ANN ini WRAPPER terpisah, opsional. Jalur search live TETAP flat (terbukti,
// recall@10=0.985). ANN = kapabilitas SIAP, di-flip nanti pas korpus nyentuh jutaan (dgn
// flat sbg fallback). Pure-Go, no-cgo, portable.

package vecindex

import (
	"math"
	"sort"
)

// ANNIndex — wrapper IVF di atas Index flat. centroids = float32 (presisi k-means mean).
type ANNIndex struct {
	ix        *Index
	nClusters int
	centroids []float32 // nClusters*dim
	members   [][]int   // per cluster: index baris ke ix
}

const kmeansIters = 6

func dotI8(a, b []int8) int32 {
	var d int32
	for j := range a {
		d += int32(a[j]) * int32(b[j])
	}
	return d
}

// assignCluster — cluster terdekat (max dot) buat baris i (codes) vs centroids float.
func assignCluster(code []int8, centroids []float32, nClusters, dim int) int {
	best, bestScore := 0, float32(-math.MaxFloat32)
	for c := 0; c < nClusters; c++ {
		cen := centroids[c*dim : (c+1)*dim]
		var d float32
		for j := range code {
			d += float32(code[j]) * cen[j]
		}
		if d > bestScore {
			bestScore, best = d, c
		}
	}
	return best
}

// BuildANN — k-means (dot/spherical, kmeansIters pass) → nClusters cluster. nClusters<=0 →
// auto ~sqrt(N). Deterministik (init evenly-spaced, no rng). ADDITIVE (Index flat ga disentuh).
func BuildANN(ix *Index, nClusters int) *ANNIndex {
	n := ix.Len()
	if n == 0 {
		return &ANNIndex{ix: ix, nClusters: 0}
	}
	if nClusters <= 0 {
		nClusters = int(math.Sqrt(float64(n)))
	}
	if nClusters < 1 {
		nClusters = 1
	}
	if nClusters > n {
		nClusters = n
	}
	dim := ix.dim
	// init centroids = baris evenly-spaced (sbg float).
	centroids := make([]float32, nClusters*dim)
	stride := n / nClusters
	if stride < 1 {
		stride = 1
	}
	for c := 0; c < nClusters; c++ {
		src := (c * stride) % n
		for j := 0; j < dim; j++ {
			centroids[c*dim+j] = float32(ix.codes[src*dim+j])
		}
	}
	assign := make([]int, n)
	for iter := 0; iter < kmeansIters; iter++ {
		for i := 0; i < n; i++ {
			assign[i] = assignCluster(ix.codes[i*dim:(i+1)*dim], centroids, nClusters, dim)
		}
		sums := make([]float32, nClusters*dim)
		counts := make([]int, nClusters)
		for i := 0; i < n; i++ {
			c := assign[i]
			counts[c]++
			row := ix.codes[i*dim : (i+1)*dim]
			for j := 0; j < dim; j++ {
				sums[c*dim+j] += float32(row[j])
			}
		}
		for c := 0; c < nClusters; c++ {
			if counts[c] == 0 {
				continue // centroid kosong → biarin (jarang ke-probe)
			}
			inv := 1.0 / float32(counts[c])
			for j := 0; j < dim; j++ {
				centroids[c*dim+j] = sums[c*dim+j] * inv
			}
		}
	}
	members := make([][]int, nClusters)
	for i := 0; i < n; i++ {
		c := assignCluster(ix.codes[i*dim:(i+1)*dim], centroids, nClusters, dim)
		members[c] = append(members[c], i)
	}
	return &ANNIndex{ix: ix, nClusters: nClusters, centroids: centroids, members: members}
}

// Len — jumlah vektor (delegasi).
func (a *ANNIndex) Len() int { return a.ix.Len() }

// Search — top-k approx: probe `nprobe` cluster terdekat (query float vs centroid float) →
// SearchSubset EXACT di member. nprobe<=0 → auto ~sqrt(nClusters). nprobe>=nClusters → flat.
func (a *ANNIndex) Search(query []float32, k, nprobe int) []Hit {
	if k <= 0 || a.ix.Len() == 0 || a.nClusters == 0 {
		return nil
	}
	if nprobe <= 0 {
		nprobe = int(math.Sqrt(float64(a.nClusters)))
		if nprobe < 1 {
			nprobe = 1
		}
	}
	if nprobe > a.nClusters {
		nprobe = a.nClusters
	}
	dim := a.ix.dim
	type cscore struct {
		c     int
		score float32
	}
	cents := make([]cscore, a.nClusters)
	for c := 0; c < a.nClusters; c++ {
		cen := a.centroids[c*dim : (c+1)*dim]
		var d float32
		for j := 0; j < dim && j < len(query); j++ {
			d += query[j] * cen[j]
		}
		cents[c] = cscore{c, d}
	}
	sort.Slice(cents, func(i, j int) bool { return cents[i].score > cents[j].score })
	cand := make([]int, 0, 1024)
	for p := 0; p < nprobe; p++ {
		cand = append(cand, a.members[cents[p].c]...)
	}
	return a.ix.SearchSubset(query, cand, k)
}
