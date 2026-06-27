// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// ratelimit_route_ext.go — SEAM (NON-frozen, DELETABLE). Wire endpoint sadar-kuota ke router
// HTTP via RegisterExtraRoute (routes_ext.go). State-nya di internal/router (1 sumber share).
//   GET /api/router/ratelimit → {util_5h, reset_5h, surpassed_5h, util_7d, fallback_pct, seen}
package main

import (
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/router"
)

func init() {
	RegisterExtraRoute(func(mux *http.ServeMux) {
		mux.HandleFunc("/api/router/ratelimit", router.RateLimitHandler)
	})
}
