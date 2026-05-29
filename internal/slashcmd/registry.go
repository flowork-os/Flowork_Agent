// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 14 phase 1 (Registry). API stable: Register (panic on
//   dup name OR alias collision — early bug catch), Lookup (resolve name
//   atau alias), List, Count, ListSummaries. sync.RWMutex thread-safe.
//
// registry.go — singleton registry slash commands.
//
// Same pattern dengan tools/registry.go: Register, Lookup (incl. alias
// resolution), List, Count.

package slashcmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	regMu       sync.RWMutex
	registry    = map[string]SlashCommand{} // canonical name → cmd
	aliasLookup = map[string]string{}        // alias → canonical name
)

// Register — add command to registry. Panic on duplicate name OR alias
// collision dengan existing name/alias (early bug catch).
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
	// Index aliases.
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

// Lookup — fetch command by name OR alias (case-insensitive).
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

// List — return semua canonical commands sorted by name.
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

// Count — total registered commands (canonical, alias tidak di-count).
func Count() int {
	regMu.RLock()
	defer regMu.RUnlock()
	return len(registry)
}

// Summary — minimal payload buat /help list.
type Summary struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
}

// ListSummaries — slice of Summary, sorted by name.
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
