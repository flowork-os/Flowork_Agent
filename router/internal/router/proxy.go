// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"context"
	"hash/fnv"
	"math/rand"
	"net/http"
	"net/url"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	proxyMu     sync.Mutex
	clientCache = map[string]*http.Client{}
	proxyCursor = map[string]int{}
)

func outboundClient(ctx context.Context) *http.Client {
	if u := pickProxyURL(ctx); u != "" {
		return clientForProxy(u)
	}
	return httpClient
}

func OutboundClient(ctx context.Context) *http.Client { return outboundClient(ctx) }

func pickProxyURL(ctx context.Context) string {
	d, err := store.Open()
	if err != nil {
		return ""
	}
	pools, err := store.ListProxyPools(d)
	if err != nil {
		return ""
	}
	for _, p := range pools {
		if !p.IsActive || len(p.Proxies) == 0 {
			continue
		}
		switch p.Rotation {
		case store.ProxyRotationRandom:
			return p.Proxies[rand.Intn(len(p.Proxies))]
		case store.ProxyRotationSticky:

			key := clientIdentity(ctx)
			if key == "" {
				return p.Proxies[0]
			}
			h := fnv.New32a()
			_, _ = h.Write([]byte(key))
			return p.Proxies[int(h.Sum32())%len(p.Proxies)]
		default:
			proxyMu.Lock()
			i := proxyCursor[p.ID] % len(p.Proxies)
			proxyCursor[p.ID] = (i + 1) % len(p.Proxies)
			proxyMu.Unlock()
			return p.Proxies[i]
		}
	}
	return ""
}

func clientForProxy(proxyURL string) *http.Client {
	proxyMu.Lock()
	defer proxyMu.Unlock()
	if c, ok := clientCache[proxyURL]; ok {
		return c
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return httpClient
	}
	c := &http.Client{
		Timeout:   httpTimeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(u)},
	}
	clientCache[proxyURL] = c
	return c
}
