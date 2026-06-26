package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouteSeamWired membuktikan seam evolusi rute (routes_ext.go) beneran ter-wire:
// RegisterExtraRoute → registerExtraRoutes(mux) → endpoint hidup. Inilah jaminan AI
// masa depan bisa nambah endpoint tanpa buka file frozen. Kalau test ini gagal,
// berarti hook registerExtraRoutes(mux) di routes.go (frozen) hilang/putus.
func TestRouteSeamWired(t *testing.T) {
	RegisterExtraRoute(func(mux *http.ServeMux) {
		mux.HandleFunc("/api/_seamtest/ping", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"seam":"ok"}`))
		})
	})
	mux := http.NewServeMux()
	registerExtraRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/_seamtest/ping")
	if err != nil {
		t.Fatalf("seam request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seam route NOT wired: status %d (registerExtraRoutes hook putus?)", resp.StatusCode)
	}
}
