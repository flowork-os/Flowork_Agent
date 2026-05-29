// Package mesh — kernel/mesh/peer_connect.go
//
// HTTP handshake + health probe ke remote peer kernel. Pre-flight before
// any heavy data exchange (weight pull, CRDT sync).
//
// Handshake protocol (MVP):
//   1. GET <peer>/healthz → expect 200 + JSON {service: "flowork-kernel"}
//   2. GET <peer>/v1/mesh/info → kernel version + node ID + capabilities
//
// Authentication defer Phase J: hardware_id signed token (per kernel/license/).
// MVP single-tenant: all peer = Ayah's own infra, trust by URL config.

package mesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ConnectedPeerInfo metadata remote kernel (from HTTP handshake).
// Renamed from PeerInfo to avoid conflict with packet.go PeerInfo (M3 peer_registry type).
type ConnectedPeerInfo struct {
	URL          string   `json:"url"`
	Service      string   `json:"service"`
	NodeID       string   `json:"node_id,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	WargaCount   int      `json:"warga_count,omitempty"`
	ToolCount    int      `json:"tool_count,omitempty"`
	HealthOK     bool     `json:"health_ok"`
	LatencyMs    int      `json:"latency_ms"`
}

// connectTimeout fixed conservative — peer mesh harus quick-fail kalau
// node down, jangan sampai hang di tengah loop discovery.
const connectTimeout = 5 * time.Second

// Connect run handshake + return PeerInfo. Caller pre-condition: peer.URL
// valid HTTPS/HTTP base path (no trailing slash).
//
// Errors:
//   - context cancel/timeout
//   - network unreachable
//   - HTTP non-2xx
//   - JSON parse fail
//   - service signature mismatch (peer return but bukan flowork-kernel)
func Connect(ctx context.Context, peer Peer) (ConnectedPeerInfo, error) {
	info := ConnectedPeerInfo{URL: peer.URL}
	if peer.URL == "" {
		return info, errors.New("mesh.Connect: peer URL empty")
	}

	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	started := time.Now()

	// 1. Healthz probe.
	if err := probeHealthz(cctx, peer.URL, &info); err != nil {
		return info, fmt.Errorf("mesh.Connect: healthz %s: %w", peer.URL, err)
	}
	info.LatencyMs = int(time.Since(started).Milliseconds())
	info.HealthOK = true

	// 2. Mesh info (optional — older kernel might not have endpoint, ngga fatal).
	_ = probeMeshInfo(cctx, peer.URL, &info)

	return info, nil
}

// probeHealthz GET /healthz, decode service + populate info.
func probeHealthz(ctx context.Context, baseURL string, info *ConnectedPeerInfo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("User-Agent", "flowork-kernel-mesh/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // 64 KiB cap
	var parsed struct {
		Service string `json:"service"`
		Status  string `json:"status"`
		Deps    struct {
			ToolsCount int `json:"tools_count"`
		} `json:"deps"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if !strings.HasPrefix(parsed.Service, "flowork-kernel") {
		return fmt.Errorf("service signature mismatch: got %q, want flowork-kernel*", parsed.Service)
	}
	info.Service = parsed.Service
	info.ToolCount = parsed.Deps.ToolsCount
	return nil
}

// probeMeshInfo GET /v1/mesh/info — extended metadata. Optional; older
// kernel pre-Phase E ngga ada endpoint, ngga fatal.
func probeMeshInfo(ctx context.Context, baseURL string, info *ConnectedPeerInfo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/mesh/info", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "flowork-kernel-mesh/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var parsed struct {
		NodeID       string   `json:"node_id"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
		WargaCount   int      `json:"warga_count"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	info.NodeID = parsed.NodeID
	info.Version = parsed.Version
	info.Capabilities = parsed.Capabilities
	info.WargaCount = parsed.WargaCount
	return nil
}

// ConnectAll iterate over all peers from Discover(), return slice PeerInfo
// + slice error per failed peer. Caller decide retry policy.
//
// Concurrency: serial untuk MVP. Phase F + Cron mungkin parallelize via
// goroutine pool kalau peer count > 5.
func ConnectAll(ctx context.Context) ([]ConnectedPeerInfo, []error) {
	peers, err := Discover()
	if err != nil {
		return nil, []error{fmt.Errorf("mesh.ConnectAll: discover: %w", err)}
	}
	if len(peers) == 0 {
		return nil, nil
	}

	infos := make([]ConnectedPeerInfo, 0, len(peers))
	errs := make([]error, 0)
	for _, p := range peers {
		info, err := Connect(ctx, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		infos = append(infos, info)
	}
	return infos, errs
}
