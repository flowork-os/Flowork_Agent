// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package slashcmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	regMu       sync.RWMutex
	registry    = map[string]SlashCommand{}
	aliasLookup = map[string]string{}
)

func Register(c SlashCommand) {
	if c == nil {
		panic("slashcmd.Register: nil command")
	}
	name := strings.ToLower(strings.TrimSpace(c.Name()))
	if name == "" {
		panic("slashcmd.Register: empty name")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if existing, ok := registry[name]; ok {
		panic(fmt.Sprintf("slashcmd.Register: duplicate name %q (existing=%T, new=%T)", name, existing, c))
	}
	registry[name] = c

	for _, a := range c.Aliases() {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if _, ok := registry[a]; ok {
			panic(fmt.Sprintf("slashcmd.Register: alias %q collides with existing command name", a))
		}
		if target, ok := aliasLookup[a]; ok {
			panic(fmt.Sprintf("slashcmd.Register: alias %q already maps to %q (new command %q)", a, target, name))
		}
		aliasLookup[a] = name
	}
}

func Lookup(name string) (SlashCommand, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	regMu.RLock()
	defer regMu.RUnlock()
	if c, ok := registry[name]; ok {
		return c, true
	}
	if target, ok := aliasLookup[name]; ok {
		return registry[target], true
	}
	return nil, false
}

func List() []SlashCommand {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]SlashCommand, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func Count() int {
	regMu.RLock()
	defer regMu.RUnlock()
	return len(registry)
}

type Summary struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
}

func ListSummaries() []Summary {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Summary, 0, len(registry))
	for _, c := range registry {
		out = append(out, Summary{
			Name:        c.Name(),
			Aliases:     c.Aliases(),
			Description: c.Description(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
