// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

const (
	tunnelWatchdogInterval  = 60 * time.Second
	tunnelWatchdogProbeWait = 6 * time.Second
)

var (
	tunnelWatchdogStarted bool
	tunnelWatchdogMu      sync.Mutex
)

func startTunnelWatchdog() {
	tunnelWatchdogMu.Lock()
	defer tunnelWatchdogMu.Unlock()
	if tunnelWatchdogStarted {
		return
	}
	tunnelWatchdogStarted = true
	go tunnelWatchdogLoop(context.Background())
}

func tunnelWatchdogLoop(ctx context.Context) {
	ticker := time.NewTicker(tunnelWatchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tunnelWatchdogTick(ctx)
		}
	}
}

func tunnelWatchdogTick(ctx context.Context) {
	d, err := store.Open()
	if err != nil {
		return
	}
	st, _ := store.LoadTunnelState(d)
	if st == nil {
		return
	}

	if st.CloudflareEnabled && st.CloudflareURL != "" {
		if !probeURLOK(ctx, st.CloudflareURL) && !isCloudflaredRunning() {
			log.Printf("flow_router tunnel watchdog: cloudflared down (url=%s)", st.CloudflareURL)
			st.CloudflareEnabled = false
			st.CloudflareURL = ""
			st.CloudflarePID = 0
			_ = store.SaveTunnelState(d, st)
		}
	}
}

func probeURLOK(ctx context.Context, base string) bool {
	if base == "" {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, tunnelWatchdogProbeWait)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return false
	}
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
