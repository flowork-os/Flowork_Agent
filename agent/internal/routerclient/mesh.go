// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package routerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type MeshIdentity struct {
	PubKey    string `json:"pubkey"`
	Hostname  string `json:"hostname"`
	Version   string `json:"version"`
	PeerCount int    `json:"peer_count"`
}

type MeshPeer struct {
	PubKeyHex   string  `json:"pubkey_hex"`
	Hostname    string  `json:"hostname"`
	IP          string  `json:"ip"`
	Port        int     `json:"port"`
	Version     string  `json:"version"`
	IsVirt      bool    `json:"is_virt"`
	FirstSeenAt string  `json:"first_seen_at"`
	LastSeenAt  string  `json:"last_seen_at"`
	TrustScore  float64 `json:"trust_score"`
	Blocked     bool    `json:"blocked"`
}

func (c *Client) Identity(ctx context.Context) (MeshIdentity, error) {
	var out MeshIdentity
	if err := c.getJSON(ctx, "/api/mesh/identity", &out); err != nil {
		return MeshIdentity{}, err
	}
	return out, nil
}

func (c *Client) ListPeers(ctx context.Context, includeBlocked bool) ([]MeshPeer, error) {
	path := "/api/mesh/peers"
	if includeBlocked {
		path += "?include_blocked=1"
	}
	var resp struct {
		Peers []MeshPeer `json:"peers"`
		Count int        `json:"count"`
	}
	if err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Peers, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	if c == nil {
		return fmt.Errorf("router client nil")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("mesh %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("router status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if jerr := json.Unmarshal(body, out); jerr != nil {
		return fmt.Errorf("decode: %w", jerr)
	}
	return nil
}
