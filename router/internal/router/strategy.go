// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"math/rand"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	rrMu             sync.Mutex
	roundRobinCursor = map[string]int{}
)

func nextRoundRobin(key string, n int) int {
	if n <= 0 {
		return 0
	}
	rrMu.Lock()
	defer rrMu.Unlock()
	i := roundRobinCursor[key] % n
	roundRobinCursor[key] = (i + 1) % n
	return i
}

func applyFallbackStrategy(matches []store.ProviderConnection, strategy, model string) []store.ProviderConnection {
	if len(matches) <= 1 {
		return matches
	}
	switch strategy {
	case "round_robin":
		i := nextRoundRobin("fb:"+model, len(matches))
		out := make([]store.ProviderConnection, 0, len(matches))
		out = append(out, matches[i:]...)
		out = append(out, matches[:i]...)
		return out
	case "random":
		out := append([]store.ProviderConnection(nil), matches...)
		rand.Shuffle(len(out), func(a, b int) { out[a], out[b] = out[b], out[a] })
		return out
	default:
		return matches
	}
}
