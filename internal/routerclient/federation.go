// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B6 federation. Verified: promote local->router shared (findable
//   by others), quality-gate (quarantine excluded), anti double-promote, resilient
//   (router down graceful). Extend -> file baru, JANGAN modify ini.
//
// federation.go — Roadmap 2 Fase B6: promote drawer lokal → router shared brain.
//
// routerclient.go (LOCKED) nyuruh extend via NEW file. File ini nambah
// PromoteDrawer: POST 1 drawer ke Router /api/brain/drawer (bring-your-own-
// knowledge). Dipakai federation: agent share knowledge ke korpus bareng.
// Resilient by caller — kalau router mati, Do() error → caller skip, agent jalan.

package routerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PromoteDrawerReq — payload /api/brain/drawer (field `memType` camelCase, sesuai
// brainAddDrawerHandler router).
type PromoteDrawerReq struct {
	Content string `json:"content"`
	Wing    string `json:"wing"`
	Room    string `json:"room"`
	MemType string `json:"memType"`
}

// PromoteDrawerResp — return router: id global + added (false = udah ada, dedup).
type PromoteDrawerResp struct {
	ID    string `json:"id"`
	Added bool   `json:"added"`
	Error string `json:"error,omitempty"`
}

// PromoteDrawer POST 1 drawer ke router shared brain. Return remote id.
func (c *Client) PromoteDrawer(ctx context.Context, req PromoteDrawerReq) (PromoteDrawerResp, error) {
	if c == nil {
		return PromoteDrawerResp{}, fmt.Errorf("router client nil")
	}
	url := c.BaseURL + "/api/brain/drawer"
	bodyJSON, err := json.Marshal(req)
	if err != nil {
		return PromoteDrawerResp{}, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return PromoteDrawerResp{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return PromoteDrawerResp{}, fmt.Errorf("promote drawer: %w", err) // router mati → caller handle
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var out PromoteDrawerResp
	if uerr := json.Unmarshal(respBytes, &out); uerr != nil {
		return PromoteDrawerResp{}, fmt.Errorf("decode (status=%d): %w", resp.StatusCode, uerr)
	}
	if resp.StatusCode >= 400 {
		if out.Error == "" {
			out.Error = fmt.Sprintf("router status %d", resp.StatusCode)
		}
		return out, fmt.Errorf("router error: %s", out.Error)
	}
	return out, nil
}
