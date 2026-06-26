// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

func discoverOIDC(ctx context.Context, issuer string) (*oidcDiscovery, error) {
	u := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery %d", resp.StatusCode)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

func oidcConfigFromSettings(s *store.Settings) (issuer, clientID, clientSecret, redirectURI, scopes string) {
	if s == nil || s.OidcConfig == nil {
		return
	}
	get := func(k string) string {
		if v, ok := s.OidcConfig[k].(string); ok {
			return v
		}
		return ""
	}
	issuer = get("issuer")
	clientID = get("clientId")
	clientSecret = get("clientSecret")
	redirectURI = get("redirectUri")
	scopes = get("scopes")
	if scopes == "" {
		scopes = "openid profile email"
	}
	if redirectURI == "" {

		scheme := os.Getenv("FLOW_ROUTER_SCHEME")
		if scheme == "" {
			scheme = "https"
		}
		host := os.Getenv("FLOW_ROUTER_HOST")
		if host == "" {
			host = "127.0.0.1:2402"
		}
		redirectURI = scheme + "://" + host + "/api/auth/oidc/callback"
	}
	return
}

func authOIDCInitHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	settings, _ := store.LoadSettings(d)
	if settings == nil || settings.AuthMode != "oidc" {
		http.Error(w, "OIDC not configured (set authMode=oidc + oidcConfig)", http.StatusBadRequest)
		return
	}
	issuer, clientID, _, redirectURI, scopes := oidcConfigFromSettings(settings)
	if issuer == "" || clientID == "" {
		http.Error(w, "oidcConfig requires issuer + clientId", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	disc, err := discoverOIDC(ctx, issuer)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "discovery: " + err.Error()})
		return
	}

	stateB := make([]byte, 32)
	_, _ = rand.Read(stateB)
	state := hex.EncodeToString(stateB)
	nonceB := make([]byte, 32)
	_, _ = rand.Read(nonceB)
	nonce := hex.EncodeToString(nonceB)

	_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
		Provider:  "oidc:pending",
		TokenType: "oidc-pending",
		Extra: map[string]any{
			"state": state, "nonce": nonce, "issuer": issuer,
			"tokenEndpoint": disc.TokenEndpoint, "userinfoEndpoint": disc.UserinfoEndpoint,
			"jwksUri":   disc.JwksURI,
			"expiresAt": time.Now().Add(10 * time.Minute).Format(time.RFC3339),
		},
	})
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scopes)
	q.Set("state", state)
	q.Set("nonce", nonce)
	authURL := disc.AuthorizationEndpoint + "?" + q.Encode()
	writeJSON(w, http.StatusOK, map[string]any{"authUrl": authURL, "state": state})
}

func authOIDCCallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	d, _ := store.Open()
	settings, _ := store.LoadSettings(d)
	issuer, clientID, clientSecret, redirectURI, _ := oidcConfigFromSettings(settings)
	_ = issuer
	pending, _ := store.GetOAuthToken(d, "oidc:pending")
	if pending == nil {
		http.Error(w, "no pending OIDC init", http.StatusBadRequest)
		return
	}
	extra, _ := pending.Extra.(map[string]any)
	storedState, _ := extra["state"].(string)

	if extra == nil || subtle.ConstantTimeCompare([]byte(storedState), []byte(state)) != 1 {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}

	if expStr, _ := extra["expiresAt"].(string); expStr != "" {
		if exp, perr := time.Parse(time.RFC3339, expStr); perr == nil && time.Now().After(exp) {
			_ = store.DeleteOAuthToken(d, "oidc:pending")
			http.Error(w, "login state expired — please retry", http.StatusBadRequest)
			return
		}
	}
	tokenEndpoint, _ := extra["tokenEndpoint"].(string)
	userinfoEndpoint, _ := extra["userinfoEndpoint"].(string)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURI)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		http.Error(w, "token exchange: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tok)
	if tok.AccessToken == "" {
		http.Error(w, "no access_token from IdP", http.StatusBadGateway)
		return
	}

	jwksURI, _ := extra["jwksUri"].(string)
	expectIssuer, _ := extra["issuer"].(string)
	nonce, _ := extra["nonce"].(string)
	var verifiedSubject string
	if tok.IDToken != "" {

		if jwksURI == "" {
			http.Error(w, "id_token present but issuer advertised no jwks_uri — cannot verify", http.StatusUnauthorized)
			return
		}
		claims, verr := verifyOIDCIDToken(ctx, jwksURI, tok.IDToken, expectIssuer, clientID, nonce)
		if verr != nil {
			http.Error(w, "id_token verification failed: "+verr.Error(), http.StatusUnauthorized)
			return
		}
		if e, ok := claims["email"].(string); ok && e != "" {
			verifiedSubject = e
		} else if s, ok := claims["sub"].(string); ok {
			verifiedSubject = s
		}
	}

	userID := "oidc-user"
	if verifiedSubject != "" {
		userID = verifiedSubject
	} else if userinfoEndpoint != "" {
		ureq, _ := http.NewRequestWithContext(ctx, http.MethodGet, userinfoEndpoint, nil)
		ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		if uresp, err := providerProbeClient.Do(ureq); err == nil {
			defer uresp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(uresp.Body, 64*1024))
			var info map[string]any
			if json.Unmarshal(body, &info) == nil {
				if sub, ok := info["email"].(string); ok && sub != "" {
					userID = sub
				} else if sub, ok := info["sub"].(string); ok {
					userID = sub
				}
			}
		}
	}
	s, err := store.CreateSession(d, userID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		http.Error(w, "session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = store.DeleteOAuthToken(d, "oidc:pending")
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: s.Token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: cookieSecure(), Expires: s.ExpiresAt,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func authEnforceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !pathRequiresSession(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		d, err := store.Open()
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		settings, _ := store.LoadSettings(d)
		if settings == nil || !settings.RequireLogin || settings.AuthMode == "none" {
			next.ServeHTTP(w, r)
			return
		}
		token := extractAuthToken(r)
		if token != "" {
			if s, err := store.GetSessionByToken(d, token); err == nil && s != nil {
				_ = store.TouchSession(d, s.ID)
				next.ServeHTTP(w, r)
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error":        "authentication required",
				"requireLogin": true,
				"authMode":     settings.AuthMode,
			})
			return
		}

		http.Redirect(w, r, "/?login=1", http.StatusFound)
	})
}

func pathRequiresSession(p string) bool {
	exempt := []string{
		"/api/auth/",
		"/v1/",
		"/v1beta/",
		"/healthz",
		"/api/health",
		"/api/shutdown",
		"/favicon",
		"/static/",
	}
	for _, e := range exempt {
		if strings.HasPrefix(p, e) {
			return false
		}
	}

	if p == "/api/mesh/packet" {
		return false
	}

	if p == "/" {
		return false
	}
	return true
}
