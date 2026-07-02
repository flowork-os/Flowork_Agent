package main

import (
	"os"
	"testing"
)

// TestMain — ISOLASI: arahin FLOW_ROUTER_DATA ke dir temp SEBELUM store.Open
// (singleton sync.Once) kepanggil → test package main NGGA nyampah/nimpa DB
// router ASLI (~/.flow_router). Wajib: sebelumnya TestAntigravityInjectHeaders
// nulis token palsu ke DB asli. Set di sini = semua test ikut aman.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "flowrouter-test-*")
	if err == nil {
		_ = os.Setenv("FLOW_ROUTER_DATA", dir)
		defer os.RemoveAll(dir)
	}
	os.Exit(m.Run())
}
