// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package utils

import (
	"net/url"
	"os"
	"strings"
)

const DefaultURL = "http://127.0.0.1:2402"

func Resolve(override string) string {
	candidates := []string{override, os.Getenv("FLOW_ROUTER_URL"), DefaultURL}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if u, err := url.Parse(c); err == nil && u.Scheme != "" && u.Host != "" {
			return strings.TrimRight(c, "/")
		}
	}
	return DefaultURL
}

func ResolveKey(override string) string {
	if override != "" {
		return override
	}
	return os.Getenv("FLOW_ROUTER_KEY")
}
