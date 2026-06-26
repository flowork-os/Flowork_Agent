// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package creds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultClaudeClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	defaultClaudeTokenURL = "https://console.anthropic.com/v1/oauth/token"

	defaultClaudeUserAgent = "claude-cli/1.0.0 (flow_router)"
)

func claudeUserAgent() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_USER_AGENT")); v != "" {
		return v
	}
	return defaultClaudeUserAgent
}

var refreshMu sync.Mutex

func claudeClientID() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_CLIENT_ID")); v != "" {
		return v
	}
	return defaultClaudeClientID
}

func claudeTokenURL() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_TOKEN_URL")); v != "" {
		return v
	}
	return defaultClaudeTokenURL
}

func LoadValid() (*CredentialsFile, error) {
	c, err := Load()
	if err != nil {
		return nil, err
	}
	if !c.IsExpired() {
		return c, nil
	}
	rt := strings.TrimSpace(c.ClaudeAiOauth.RefreshToken)
	if rt == "" {
		return nil, fmt.Errorf("claude token expired and no refresh token available — re-import the token via OAuth Imports → Browse")
	}

	refreshMu.Lock()
	defer refreshMu.Unlock()
	if c2, e := Load(); e == nil && !c2.IsExpired() {
		return c2, nil
	}

	access, refresh, expMs, rerr := refreshClaude(rt)
	if rerr != nil {
		return nil, fmt.Errorf("claude token expired and refresh failed (%v) — re-import via OAuth Imports → Browse", rerr)
	}

	expStr := ""
	if expMs > 0 {
		expStr = strconv.FormatInt(expMs, 10)
	}
	if serr := SaveClaude(access, refresh, expStr); serr != nil {
		return nil, fmt.Errorf("claude token refreshed but persisting it failed: %w", serr)
	}
	InvalidateCache()
	return Load()
}

func refreshClaude(refreshToken string) (access, refresh string, expiresAtMs int64, err error) {
	return postClaudeToken(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     claudeClientID(),
	})
}

func postClaudeToken(payload map[string]string) (access, refresh string, expiresAtMs int64, err error) {
	reqBody, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, claudeTokenURL(), bytes.NewReader(reqBody))
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	req.Header.Set("User-Agent", claudeUserAgent())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {

		return "", "", 0, fmt.Errorf("token endpoint HTTP %d", resp.StatusCode)
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if e := json.NewDecoder(resp.Body).Decode(&out); e != nil {
		return "", "", 0, e
	}
	if out.AccessToken == "" {
		return "", "", 0, fmt.Errorf("token endpoint returned no access_token")
	}
	if out.RefreshToken == "" {
		out.RefreshToken = payload["refresh_token"]
	}
	if out.ExpiresIn > 0 {
		expiresAtMs = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second).UnixMilli()
	}
	return out.AccessToken, out.RefreshToken, expiresAtMs, nil
}
