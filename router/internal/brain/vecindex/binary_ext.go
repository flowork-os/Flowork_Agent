// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package vecindex

import (
	"math/bits"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type binData struct {
	words int
	sigs  []uint64
}

var binCache sync.Map

func binaryMinN() int {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_BINARY_VECTOR_MIN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1_000_000
}

func (ix *Index) useBinary() bool {
	if ix.dim <= 0 || ix.Len() < 256 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_BINARY_VECTOR"))) {
	case "off", "0", "false":
		return false
	case "on", "1", "true", "force":
		return true
	}
	return ix.Len() >= binaryMinN()
}

func (ix *Index) binSigs() *binData {
	if v, ok := binCache.Load(ix); ok {
		return v.(*binData)
	}
	words := (ix.dim + 63) / 64
	n := ix.Len()
	sigs := make([]uint64, n*words)
	workers := runtime.NumCPU()
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lo, hi := w*chunk, w*chunk+chunk
		if hi > n {
			hi = n
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				row := ix.codes[i*ix.dim : (i+1)*ix.dim]
				base := i * words
				for j, c := range row {
					if c > 0 {
						sigs[base+j/64] |= 1 << uint(j%64)
					}
				}
			}
		}(lo, hi)
	}
	wg.Wait()
	bd := &binData{words: words, sigs: sigs}
	binCache.Store(ix, bd)
	return bd
}

func signQuery(q []int8, words int) []uint64 {
	out := make([]uint64, words)
	for j, c := range q {
		if c > 0 {
			out[j/64] |= 1 << uint(j%64)
		}
	}
	return out
}

func (ix *Index) searchBinary(query []float32, k int) []Hit {
	bd := ix.binSigs()
	q := make([]int8, ix.dim)
	quantizeInto(query, ix.scale, q)
	qs := signQuery(q, bd.words)

	M := k * 64
	if M < 2000 {
		M = 2000
	}
	if M > ix.Len() {
		M = ix.Len()
	}
	n := ix.Len()
	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}
	partial := make([][]scored, workers)
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lo, hi := w*chunk, w*chunk+chunk
		if hi > n {
			hi = n
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(w, lo, hi int) {
			defer wg.Done()
			top := make([]scored, 0, M)
			for i := lo; i < hi; i++ {
				base := i * bd.words
				var agree int32
				for wi := 0; wi < bd.words; wi++ {

					agree += int32(bits.OnesCount64(^(qs[wi] ^ bd.sigs[base+wi])))
				}
				top = pushTopK(top, M, scored{i, agree})
			}
			partial[w] = top
		}(w, lo, hi)
	}
	wg.Wait()
	cand := make([]int, 0, workers*M)
	for _, p := range partial {
		for _, s := range p {
			cand = append(cand, s.idx)
		}
	}

	hits := ix.SearchSubset(query, cand, k)
	sort.Slice(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	return hits
}
