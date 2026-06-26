// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mcpsecurity

import (
	"path/filepath"
	"strings"
	"sync"
)

var defaultAllowed = []string{
	"npx",
	"node",
	"uvx",
	"python",
	"python3",
	"bunx",
	"bun",
	"deno",
	"pnpm",
	"yarn",
}

var (
	mu      sync.RWMutex
	allowed = makeAllowedSet(defaultAllowed)
)

func makeAllowedSet(list []string) map[string]struct{} {
	out := make(map[string]struct{}, len(list))
	for _, p := range list {
		out[strings.ToLower(strings.TrimSpace(p))] = struct{}{}
	}
	return out
}

func IsAllowed(command string) bool {
	if command == "" {
		return false
	}

	if strings.Contains(command, "..") {
		return false
	}

	normalised := strings.ReplaceAll(command, "\\", "/")
	base := filepath.Base(normalised)

	lower := strings.ToLower(base)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		if strings.HasSuffix(lower, ext) {
			lower = lower[:len(lower)-len(ext)]
			break
		}
	}

	mu.RLock()
	defer mu.RUnlock()
	_, ok := allowed[lower]
	return ok
}

func Allow(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	allowed[name] = struct{}{}
}

func Set(list []string) {
	mu.Lock()
	defer mu.Unlock()
	allowed = makeAllowedSet(list)
}

func Defaults() []string {
	out := make([]string, len(defaultAllowed))
	copy(out, defaultAllowed)
	return out
}

func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(allowed))
	for k := range allowed {
		out = append(out, k)
	}
	return out
}
