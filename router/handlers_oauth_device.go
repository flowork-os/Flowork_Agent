// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: OAuth Imports → dok lock/gui/OAuth Imports.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func oauthDeviceStartHandler(w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ClientID      string `json:"clientId"`
		Scope         string `json:"scope"`
		DeviceAuthURL string `json:"deviceAuthUrl"`
		TokenURL      string `json:"tokenUrl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	tpl := oauthTemplates[provider]
	deviceURL := firstNonEmptyStr(body.DeviceAuthURL, tpl.DeviceAuthURL)
	tokenURL := firstNonEmptyStr(body.TokenURL, tpl.TokenURL)
	clientID := firstNonEmptyStr(body.ClientID, "PLACEHOLDER_"+tpl.ClientIDEnv)
	scope := firstNonEmptyStr(body.Scope, tpl.DefaultScope)
	if deviceURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": provider + " has no device_authorization endpoint — pass deviceAuthUrl"})
		return
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	if scope != "" {
		form.Set("scope", scope)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, deviceURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "device-code: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var dc struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		Interval                int    `json:"interval"`
		ExpiresIn               int    `json:"expires_in"`
	}
	if json.Unmarshal(raw, &dc) != nil || dc.DeviceCode == "" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "device endpoint returned no device_code", "raw": truncateStr(string(raw), 200)})
		return
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	d, _ := store.Open()
	_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
		Provider:  provider + ":device-pending",
		TokenType: "device-pending",
		Extra: map[string]any{
			"deviceCode": dc.DeviceCode, "tokenUrl": tokenURL, "clientId": clientID,
			"interval": dc.Interval, "expiresAt": time.Now().Add(time.Duration(clampDeviceExpiresIn(dc.ExpiresIn)) * time.Second).Format(time.RFC3339),
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"userCode": dc.UserCode, "verificationUri": dc.VerificationURI,
		"verificationUriComplete": dc.VerificationURIComplete, "interval": dc.Interval, "expiresIn": dc.ExpiresIn,
	})
}

func oauthDevicePollHandler(w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	pending, _ := store.GetOAuthToken(d, provider+":device-pending")
	if pending == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": "no device-code flow in progress"})
		return
	}
	extra, _ := pending.Extra.(map[string]any)
	deviceCode, _ := extra["deviceCode"].(string)
	tokenURL, _ := extra["tokenUrl"].(string)
	clientID, _ := extra["clientId"].(string)
	if tokenURL == "" || deviceCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "error": "incomplete pending state"})
		return
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", clientID)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"status": "error", "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(raw, &tok)
	if tok.AccessToken != "" {
		_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
			Provider: provider, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
			TokenType: firstNonEmptyStr(tok.TokenType, "Bearer"), Scope: tok.Scope,
		})
		_ = store.DeleteOAuthToken(d, provider+":device-pending")
		writeJSON(w, http.StatusOK, map[string]any{"status": "complete", "provider": provider})
		return
	}
	switch tok.Error {
	case "authorization_pending", "":
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
	case "slow_down":
		writeJSON(w, http.StatusOK, map[string]any{"status": "slow_down"})
	default:
		_ = store.DeleteOAuthToken(d, provider+":device-pending")
		writeJSON(w, http.StatusOK, map[string]any{"status": "error", "error": tok.Error})
	}
}

func clampDeviceExpiresIn(v int) int {
	const min = 600
	const max = 24 * 60 * 60
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
