// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"context"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type ctxKeyAPIKeyType struct{}

var ctxKeyAPIKey = ctxKeyAPIKeyType{}

func WithAPIKey(ctx context.Context, k *store.APIKey) context.Context {
	return context.WithValue(ctx, ctxKeyAPIKey, k)
}

func APIKeyFromContext(ctx context.Context) *store.APIKey {
	k, _ := ctx.Value(ctxKeyAPIKey).(*store.APIKey)
	return k
}

func filterByAllowedProviders(matches []store.ProviderConnection, k *store.APIKey) []store.ProviderConnection {
	if k == nil {
		return matches
	}
	allow := strings.TrimSpace(k.AllowedProviders)
	if allow == "" || allow == "*" {
		return matches
	}
	allowed := map[string]bool{}
	for _, a := range strings.Split(allow, ",") {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			allowed[a] = true
		}
	}
	var out []store.ProviderConnection
	for _, p := range matches {
		if allowed[strings.ToLower(p.Provider)] || allowed[strings.ToLower(p.Name)] {
			out = append(out, p)
		}
	}
	return out
}

func apiKeyID(ctx context.Context) string {
	if k := APIKeyFromContext(ctx); k != nil {
		return k.ID
	}
	return ""
}

type ctxKeyClientIPType struct{}

var ctxKeyClientIP = ctxKeyClientIPType{}

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ctxKeyClientIP, ip)
}

func clientIdentity(ctx context.Context) string {
	if ip, _ := ctx.Value(ctxKeyClientIP).(string); ip != "" {
		return ip
	}
	return apiKeyID(ctx)
}
