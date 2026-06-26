// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package provider

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ProviderConfig struct {
	Name     string
	BaseURL  string
	APIKey   string
	AuthType string
}

type ChatRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

type ChatResponse struct {
	Provider  string                   `json:"_provider"`
	Model     string                   `json:"model"`
	Choices   []map[string]interface{} `json:"choices"`
	Usage     UsageInfo                `json:"usage"`
	LatencyMS int64                    `json:"_latency_ms"`
	Raw       json.RawMessage          `json:"-"`
}

type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChainOrchestrator struct {
	db  *sql.DB
	cli *http.Client
}

func NewChainOrchestrator(db *sql.DB) *ChainOrchestrator {
	return &ChainOrchestrator{
		db:  db,
		cli: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *ChainOrchestrator) Run(ctx context.Context, chainName string, req ChatRequest) (*ChatResponse, error) {
	providers, err := c.resolveChain(chainName)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("chain %q has no providers", chainName)
	}
	var lastErr error
	for _, p := range providers {
		resp, ferr := c.invoke(ctx, p, req)
		if ferr == nil {
			return resp, nil
		}
		lastErr = ferr
		if !shouldFallback(ferr) {
			return nil, ferr
		}
	}
	return nil, fmt.Errorf("all providers in chain %q failed: %w", chainName, lastErr)
}

func (c *ChainOrchestrator) resolveChain(chainName string) ([]ProviderConfig, error) {
	var providersJSON string
	err := c.db.QueryRow(
		`SELECT providers_json FROM provider_chain_configs WHERE chain_name = ?`,
		chainName).Scan(&providersJSON)
	if err == sql.ErrNoRows {

		providersJSON = `["default"]`
	} else if err != nil {
		return nil, err
	}
	var providerIDs []string
	if perr := json.Unmarshal([]byte(providersJSON), &providerIDs); perr != nil {
		return nil, fmt.Errorf("invalid providers_json: %w", perr)
	}
	out := []ProviderConfig{}
	for _, pid := range providerIDs {
		pc, perr := c.loadProvider(pid)
		if perr != nil {
			continue
		}
		out = append(out, pc)
	}
	return out, nil
}

func (c *ChainOrchestrator) loadProvider(name string) (ProviderConfig, error) {
	var pc ProviderConfig
	pc.Name = name
	err := c.db.QueryRow(
		`SELECT
		   COALESCE(baseUrl, ''),
		   COALESCE(authValue, ''),
		   COALESCE(authType, 'bearer')
		 FROM providerConnections
		 WHERE name = ? AND isActive = 1`, name).Scan(&pc.BaseURL, &pc.APIKey, &pc.AuthType)
	if err == sql.ErrNoRows {
		return pc, fmt.Errorf("provider %q not found", name)
	}
	return pc, err
}

func (c *ChainOrchestrator) invoke(ctx context.Context, p ProviderConfig, req ChatRequest) (*ChatResponse, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("provider %q missing baseUrl", p.Name)
	}
	url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	bodyJSON, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		switch p.AuthType {
		case "apikey":
			httpReq.Header.Set("x-api-key", p.APIKey)
		case "subscription":
			httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		default:
			httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		}
	}
	t0 := time.Now()
	resp, err := c.cli.Do(httpReq)
	latency := time.Since(t0).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("provider %q: %w", p.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, fmt.Errorf("provider %q status %d: %s", p.Name, resp.StatusCode, string(body))
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	var out ChatResponse
	if jerr := json.Unmarshal(raw, &out); jerr != nil {
		return nil, fmt.Errorf("decode: %w", jerr)
	}
	out.Provider = p.Name
	out.LatencyMS = latency
	out.Raw = raw
	return &out, nil
}

func shouldFallback(err error) bool {
	msg := strings.ToLower(err.Error())
	hints := []string{"status 429", "status 500", "status 502", "status 503", "status 504",
		"connection refused", "timeout", "no route to host"}
	for _, h := range hints {
		if strings.Contains(msg, h) {
			return true
		}
	}
	return false
}
