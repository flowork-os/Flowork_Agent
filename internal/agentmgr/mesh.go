// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 20 phase 1 — Agent → Router mesh proxy endpoints.
//   2 endpoint siap: mesh/peers + mesh/identity. BroadcastTool /
//   FindTool / RequestKnowledge defer phase 2 sampai Router mesh
//   Section 17-19 ada. Phase 2 endpoints → tambah file baru.
//
// mesh.go — Section 20 phase 1: Agent mesh proxy.

package agentmgr

import (
	"context"
	"net/http"
	"strings"
	"time"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/routerclient"
)

// MeshIdentityHandler — GET /api/agents/mesh/identity?id=<agent>
// Proxy ke Router /api/mesh/identity via per-agent routerclient.
func MeshIdentityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	client, err := buildRouterClient(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var id routerclient.MeshIdentity
	rerr := routerclient.WithRetry(ctx, routerclient.DefaultRetry(),
		func(ctx context.Context) error {
			var ierr error
			id, ierr = client.Identity(ctx)
			return ierr
		})
	if rerr != nil {
		httpx.WriteJSON(w, map[string]any{"error": rerr.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"pubkey":     id.PubKey,
		"hostname":   id.Hostname,
		"version":    id.Version,
		"peer_count": id.PeerCount,
	})
}

// MeshPeersHandler — GET /api/agents/mesh/peers?id=<agent>&include_blocked=1
func MeshPeersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	includeBlocked := r.URL.Query().Get("include_blocked") == "1"

	client, err := buildRouterClient(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var peers []routerclient.MeshPeer
	rerr := routerclient.WithRetry(ctx, routerclient.DefaultRetry(),
		func(ctx context.Context) error {
			var ierr error
			peers, ierr = client.ListPeers(ctx, includeBlocked)
			return ierr
		})
	if rerr != nil {
		httpx.WriteJSON(w, map[string]any{"error": rerr.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"peers": peers,
		"count": len(peers),
	})
}
