// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"context"
	"net/http"
	"strings"
	"time"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/routerclient"
)

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
