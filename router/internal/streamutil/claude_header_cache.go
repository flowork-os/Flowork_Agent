// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package streamutil

import (
	"strings"
	"sync"
	"sync/atomic"
)

var ClaudeIdentityHeaders = []string{
	"user-agent",
	"anthropic-beta",
	"anthropic-version",
	"anthropic-dangerous-direct-browser-access",
	"x-app",
	"x-stainless-helper-method",
	"x-stainless-retry-count",
	"x-stainless-runtime-version",
	"x-stainless-package-version",
	"x-stainless-runtime",
	"x-stainless-lang",
	"x-stainless-arch",
	"x-stainless-os",
	"x-stainless-timeout",
	"x-claude-code-session-id",
	"package-version",
	"runtime-version",
	"os",
	"arch",
}

var (
	cachedHeaders atomic.Value
	cacheMu       sync.Mutex
)

func IsClaudeCodeClient(headers map[string]string) bool {
	ua := strings.ToLower(headers["user-agent"])
	xApp := strings.ToLower(headers["x-app"])
	return strings.Contains(ua, "claude-cli") || strings.Contains(ua, "claude-code") || xApp == "cli"
}

func CaptureFromRequest(headers map[string]string) {
	if !IsClaudeCodeClient(headers) {
		return
	}
	snapshot := map[string]string{}
	for _, k := range ClaudeIdentityHeaders {
		if v := headers[strings.ToLower(k)]; v != "" {
			snapshot[k] = v
		}
	}
	if len(snapshot) == 0 {
		return
	}
	cacheMu.Lock()
	cachedHeaders.Store(snapshot)
	cacheMu.Unlock()
}

func GetCachedClaudeHeaders() map[string]string {
	v := cachedHeaders.Load()
	if v == nil {
		return nil
	}
	m, _ := v.(map[string]string)
	if m == nil {
		return nil
	}

	out := make(map[string]string, len(m))
	for k, vv := range m {
		out[k] = vv
	}
	return out
}

func HasCachedClaudeHeaders() bool { return cachedHeaders.Load() != nil }
