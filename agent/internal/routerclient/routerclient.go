// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package routerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultRouterURL = "http://127.0.0.1:2402"

const DefaultTimeout = 30 * time.Second

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

var allowedHosts = map[string]struct{}{
	"127.0.0.1": {},
	"localhost": {},
	"0.0.0.0":   {},
}

func New(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultRouterURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if !isAllowedRouterURL(baseURL) {
		baseURL = DefaultRouterURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

func isAllowedRouterURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil || u.User != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	_, ok := allowedHosts[host]
	return ok
}

type SubmitMistakeReq struct {
	AgentID  string `json:"agent_id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	HitCount int64  `json:"hit_count"`
}

type SubmitMistakeResp struct {
	ID    int64  `json:"id"`
	Added bool   `json:"added"`
	Error string `json:"error,omitempty"`
}

func (c *Client) SubmitMistake(ctx context.Context, req SubmitMistakeReq) (SubmitMistakeResp, error) {
	if c == nil {
		return SubmitMistakeResp{}, fmt.Errorf("router client nil")
	}
	url := c.BaseURL + "/api/mistakes/submit"

	bodyJSON, err := json.Marshal(req)
	if err != nil {
		return SubmitMistakeResp{}, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return SubmitMistakeResp{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return SubmitMistakeResp{}, fmt.Errorf("submit mistake: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var out SubmitMistakeResp
	if uerr := json.Unmarshal(respBytes, &out); uerr != nil {
		return SubmitMistakeResp{}, fmt.Errorf("decode (status=%d): %w", resp.StatusCode, uerr)
	}
	if resp.StatusCode >= 400 {
		if out.Error == "" {
			out.Error = fmt.Sprintf("router status %d", resp.StatusCode)
		}
		return out, fmt.Errorf("router error: %s", out.Error)
	}
	return out, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("router client nil")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("router unhealthy status %d", resp.StatusCode)
	}
	return nil
}
