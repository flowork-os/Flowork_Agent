// Package commands mengelola slash command user-defined berbasis markdown.
//
// Owner bisa menaruh file .md di ~/.flowork/commands/<nama>.md atau di
// <workspace>/.flowork/commands/<nama>.md. Command langsung tersedia di TUI
// sebagai /<nama> tanpa rebuild.
//
// Format file:
//
//	---
//	description: Short one-liner ditampilkan di /help
//	---
//	Prompt template yang akan di-inject ke input box saat command dipanggil.
//	Placeholder {args} akan diganti argumen command (setelah nama command).
//
// Contoh (.flowork/commands/doc-check.md):
//
//	---
//	description: Audit README + AGENTS.md konsisten
//	---
//	Baca README.md dan promp/AGENTS.md. Bandingkan: bagian mana di README
//	yang bertentangan dengan AGENTS.md? Laporkan dengan file:line. Fokus
//	pada {args} kalau diisi.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/teetah2402/flowork/internal/mdloader"
)

// CommandMeta — YAML frontmatter per file.
type CommandMeta struct {
	Description string   `yaml:"description"`
	Aliases     []string `yaml:"aliases"`
}

// Command adalah hasil parse file yang siap dipanggil.
type Command struct {
	Name        string
	Description string
	Aliases     []string
	Prompt      string // body file dengan placeholder {args}
	Source      string // path file — untuk /help troubleshooting
}

// Expand substitutes {args} dalam prompt dengan user input aktual.
func (c *Command) Expand(args string) string {
	return strings.ReplaceAll(c.Prompt, "{args}", args)
}

var (
	mu       sync.RWMutex
	registry = map[string]*Command{}
)

// Load scans user-global + workspace command dirs dan mengisi registry.
// Workspace commands override user-global untuk nama yang sama.
// Called sekali di startup. Returns jumlah command yang berhasil dimuat.
//
// CODEX-BUG-18 fix: previously returned len(registry), which counted every
// alias separately — a single command with 3 aliases was reported as 4.
// Now walks the map once and counts only the distinct Command pointers.
func Load(workspace string) int {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]*Command{}

	if home, err := os.UserHomeDir(); err == nil {
		loadDir(filepath.Join(home, ".flowork", "commands"))
	}
	if workspace != "" {
		loadDir(filepath.Join(workspace, ".flowork", "commands"))
	}
	seen := map[string]bool{}
	count := 0
	for _, c := range registry {
		if !seen[c.Name] {
			seen[c.Name] = true
			count++
		}
	}
	return count
}

func loadDir(dir string) {
	for _, doc := range mdloader.LoadDir[CommandMeta](dir) {
		name := strings.TrimSuffix(strings.ToLower(filepath.Base(doc.Path)), ".md")
		cmd := &Command{
			Name:        name,
			Description: doc.Meta.Description,
			Aliases:     doc.Meta.Aliases,
			Prompt:      strings.TrimSpace(doc.Body),
			Source:      doc.Path,
		}
		registry[name] = cmd
		for _, a := range cmd.Aliases {
			registry[strings.ToLower(a)] = cmd
		}
	}
}

// Lookup finds a command by name or alias. Returns nil if unknown.
func Lookup(name string) *Command {
	mu.RLock()
	defer mu.RUnlock()
	return registry[strings.ToLower(name)]
}

// List returns distinct commands (dedupes aliases) sorted by name.
//
// CODEX-BUG-19 fix: the doc claimed "sorted by name" but the implementation
// iterated the underlying map, producing non-deterministic order across runs.
// /help and snapshot-style tests now see a stable listing.
func List() []*Command {
	mu.RLock()
	defer mu.RUnlock()
	seen := map[string]bool{}
	var out []*Command
	for _, c := range registry {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Describe formats the registry for /help output.
func Describe() string {
	cmds := List()
	if len(cmds) == 0 {
		return "(no user-defined slash commands — tambahkan file markdown di ~/.flowork/commands/ atau <workspace>/.flowork/commands/)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d user-defined slash command(s):\n\n", len(cmds))
	for _, c := range cmds {
		fmt.Fprintf(&sb, "  /%-20s %s\n", c.Name, c.Description)
	}
	return strings.TrimRight(sb.String(), "\n")
}
