// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package translator

import "sync"

type Pair struct {
	From string
	To   string
}

type Translator func(body map[string]any) map[string]any

type Direction string

const (
	DirRequest  Direction = "request"
	DirResponse Direction = "response"
)

type key struct {
	Pair Pair
	Dir  Direction
}

var (
	regMu sync.RWMutex
	reg   = map[key]Translator{}
)

func Register(pair Pair, dir Direction, fn Translator) {
	if fn == nil {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	reg[key{Pair: pair, Dir: dir}] = fn
}

func Get(from, to string, dir Direction) Translator {
	regMu.RLock()
	defer regMu.RUnlock()
	return reg[key{Pair: Pair{From: from, To: to}, Dir: dir}]
}

func List() []struct {
	From, To  string
	Direction Direction
} {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]struct {
		From, To  string
		Direction Direction
	}, 0, len(reg))
	for k := range reg {
		out = append(out, struct {
			From, To  string
			Direction Direction
		}{k.Pair.From, k.Pair.To, k.Dir})
	}
	return out
}
