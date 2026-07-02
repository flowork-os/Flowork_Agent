// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: OAuth Imports → dok lock/gui/OAuth Imports.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/creds"
	"github.com/flowork-os/flowork_Router/internal/store"
)

type oauthProviderTemplate struct {
	Provider      string   `json:"provider"`
	AuthURL       string   `json:"authUrl"`
	TokenURL      string   `json:"tokenUrl"`
	DeviceAuthURL string   `json:"deviceAuthUrl,omitempty"`
	DefaultScope  string   `json:"defaultScope"`
	ClientIDEnv   string   `json:"clientIdEnv"`
	Scopes        []string `json:"scopes"`
	Notes         string   `json:"notes,omitempty"`
}

var oauthTemplates = map[string]oauthProviderTemplate{
	"codex": {
		Provider: "codex", AuthURL: "https://auth.openai.com/authorize",
		TokenURL:     "https://auth.openai.com/oauth/token",
		DefaultScope: "openid profile email offline_access codex",
		ClientIDEnv:  "OPENAI_CODEX_CLIENT_ID",
		Scopes:       []string{"openid", "profile", "email", "offline_access", "codex"},
	},
	"cursor": {
		Provider: "cursor", AuthURL: "https://www.cursor.com/oauth/authorize",
		TokenURL:     "https://www.cursor.com/oauth/token",
		DefaultScope: "openid profile offline_access",
		ClientIDEnv:  "CURSOR_CLIENT_ID",
	},
	"gitlab": {
		Provider: "gitlab", AuthURL: "https://gitlab.com/oauth/authorize",
		TokenURL:     "https://gitlab.com/oauth/token",
		DefaultScope: "read_api read_user openid",
		ClientIDEnv:  "GITLAB_DUO_CLIENT_ID",
	},
	"iflow": {
		Provider: "iflow", AuthURL: "https://iflow.cn/oauth/authorize",
		TokenURL:     "https://iflow.cn/oauth/token",
		DefaultScope: "chat completions",
		ClientIDEnv:  "IFLOW_CLIENT_ID",
	},
	"kiro": {
		Provider: "kiro", AuthURL: "https://kiro.dev/oauth/authorize",
		TokenURL:     "https://kiro.dev/oauth/token",
		DefaultScope: "openid email free-claude-tier",
		ClientIDEnv:  "KIRO_CLIENT_ID",
		Notes:        "Free Claude Sonnet/Opus tier via Kiro signup.",
	},
	"claude": {
		Provider: "claude", AuthURL: "https://console.anthropic.com/oauth/authorize",
		TokenURL:     "https://console.anthropic.com/oauth/token",
		DefaultScope: "anthropic-completions",
		ClientIDEnv:  "ANTHROPIC_CLIENT_ID",
		Notes:        "Subscription mode currently reads ~/.claude/.credentials.json directly.",
	},

	"github": {
		Provider: "github", AuthURL: "https://github.com/login/oauth/authorize",
		TokenURL:      "https://github.com/login/oauth/access_token",
		DeviceAuthURL: "https://github.com/login/device/code",
		DefaultScope:  "read:user", ClientIDEnv: "GITHUB_CLIENT_ID",
		Notes: "GitHub Copilot uses the device-code flow.",
	},
	"xai": {
		Provider: "xai", AuthURL: "https://auth.x.ai/oauth2/authorize",
		TokenURL:     "https://auth.x.ai/oauth2/token",
		DefaultScope: "openid profile email offline_access grok-cli:access api:access",
		ClientIDEnv:  "XAI_CLIENT_ID",
		Scopes:       []string{"openid", "profile", "email", "offline_access", "grok-cli:access", "api:access"},
		Notes:        "PKCE public client; fixed loopback port 56121",
	},
	"qwen": {
		Provider: "qwen", AuthURL: "https://chat.qwen.ai/oauth/authorize",
		TokenURL:      "https://chat.qwen.ai/oauth/token",
		DeviceAuthURL: "https://chat.qwen.ai/oauth/device/code",
		DefaultScope:  "openid profile", ClientIDEnv: "QWEN_CLIENT_ID",
	},
}

func oauthRouterHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/oauth")
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		oauthListHandler(w, r)
		return
	}
	if rest == "imports" {
		oauthImportsHandler(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	provider := parts[0]
	if len(parts) == 1 {
		oauthProviderHandler(w, r, provider)
		return
	}
	sub := parts[1]
	switch {
	case sub == "init":
		oauthInitHandler(w, r, provider)
	case strings.HasPrefix(sub, "callback"):
		oauthCallbackHandler(w, r, provider)
	case sub == "social-authorize":

		oauthInitHandler(w, r, provider)
	case sub == "social-exchange":

		oauthCallbackHandler(w, r, provider)
	case sub == "device-code":
		oauthDeviceStartHandler(w, r, provider)
	case sub == "poll":
		oauthDevicePollHandler(w, r, provider)
	case sub == "import-token", sub == "import", sub == "pat", sub == "cookie":
		oauthImportActionHandler(w, r, provider, sub)
	case sub == "auto-import":
		oauthAutoImportHandler(w, r, provider)
	default:
		http.Error(w, "unknown OAuth sub-route: "+sub, http.StatusNotFound)
	}
}

func oauthImportActionHandler(w http.ResponseWriter, r *http.Request, provider, kind string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Token        string `json:"token"`
		AccessToken  string `json:"accessToken"`
		APIKey       string `json:"apiKey"`
		PAT          string `json:"pat"`
		Cookie       string `json:"cookie"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	tok := firstNonEmptyStr(body.AccessToken, body.Token, body.APIKey, body.PAT, body.Cookie)
	if tok == "" {
		http.Error(w, "credential required (token/accessToken/apiKey/pat/cookie)", http.StatusBadRequest)
		return
	}
	tokType := "Bearer"
	switch kind {
	case "pat":
		tokType = "pat"
	case "cookie":
		tokType = "cookie"
	}
	d, _ := store.Open()
	rec := &store.OAuthTokenRecord{
		Provider: provider, AccessToken: tok, RefreshToken: body.RefreshToken,
		TokenType: tokType, Scope: kind,
	}
	if err := store.UpsertOAuthToken(d, rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": provider, "kind": kind, "imported": true, "token": maskOAuthToken(rec),
	})
}

func oauthAutoImportHandler(w http.ResponseWriter, _ *http.Request, provider string) {
	statuses := creds.DetectAll()
	var found *creds.ImportStatus
	for i := range statuses {
		s := statuses[i]
		if (strings.EqualFold(s.Source, provider) || strings.Contains(strings.ToLower(s.Source), strings.ToLower(provider))) && s.Found {
			found = &s
			break
		}
	}
	if found == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": provider, "autoImported": false,
			"hint": "no local credential found — use POST /api/oauth/" + provider + " to paste a token",
		})
		return
	}

	tok, err := loadDetectedToken(provider)
	if err != nil || tok == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": provider, "autoImported": false, "source": found.Path,
			"hint": "credential detected but token not auto-parsable here — paste it via POST /api/oauth/" + provider,
		})
		return
	}
	d, _ := store.Open()
	rec := &store.OAuthTokenRecord{Provider: provider, AccessToken: tok, TokenType: "Bearer", Scope: "auto-import"}
	if err := store.UpsertOAuthToken(d, rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": provider, "autoImported": true, "source": found.Path,
		"token": maskOAuthToken(rec), "detail": "token imported from local credential",
	})
}

// extraTokenLoaders — PAPAN COLOKAN (Rule #7): loader token CLI baru (antigravity
// dll) dicolok dari NON-frozen via RegisterTokenLoader → file beku ga perlu dibuka
// tiap nambah CLI. Plug-and-play.
var extraTokenLoaders = map[string]func() (string, error){}

// RegisterTokenLoader — colok loader token buat provider (lowercase). init() sibling.
func RegisterTokenLoader(provider string, f func() (string, error)) {
	if f != nil && provider != "" {
		extraTokenLoaders[strings.ToLower(provider)] = f
	}
}

func loadDetectedToken(provider string) (string, error) {
	switch strings.ToLower(provider) {
	case "codex", "openai", "codex-cli":
		return creds.LoadCodexToken()
	case "cursor":
		return creds.LoadCursorToken()
	case "claude", "claude-code", "anthropic":
		c, err := creds.Load()
		if err != nil {
			return "", err
		}
		return c.ClaudeAiOauth.AccessToken, nil
	default:
		if f, ok := extraTokenLoaders[strings.ToLower(provider)]; ok {
			return f()
		}
		return "", fmt.Errorf("no auto-parser for %s", provider)
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func oauthListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	tokens, _ := store.ListOAuthTokens(d)
	masked := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		masked = append(masked, maskOAuthToken(&t))
	}
	templates := []oauthProviderTemplate{}
	for _, t := range oauthTemplates {
		templates = append(templates, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tokens":    masked,
		"count":     len(masked),
		"supported": templates,
	})
}

func oauthProviderHandler(w http.ResponseWriter, r *http.Request, provider string) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		t, err := store.GetOAuthToken(d, provider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if t == nil {
			writeJSON(w, http.StatusOK, map[string]any{"provider": provider, "stored": false})
			return
		}
		writeJSON(w, http.StatusOK, maskOAuthToken(t))
	case http.MethodPost:

		var body struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken,omitempty"`
			IDToken      string `json:"idToken,omitempty"`
			TokenType    string `json:"tokenType,omitempty"`
			Scope        string `json:"scope,omitempty"`
			ExpiresAt    string `json:"expiresAt,omitempty"`
			Extra        any    `json:"extra,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.AccessToken == "" {
			http.Error(w, "accessToken required", http.StatusBadRequest)
			return
		}
		if body.TokenType == "" {
			body.TokenType = "Bearer"
		}
		t := &store.OAuthTokenRecord{
			Provider:     provider,
			AccessToken:  body.AccessToken,
			RefreshToken: body.RefreshToken,
			IDToken:      body.IDToken,
			TokenType:    body.TokenType,
			Scope:        body.Scope,
			ExpiresAt:    body.ExpiresAt,
			Extra:        body.Extra,
		}
		if err := store.UpsertOAuthToken(d, t); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if provider == "claude" || provider == "anthropic" {
			if serr := creds.SaveClaude(body.AccessToken, body.RefreshToken, body.ExpiresAt); serr != nil {
				fmt.Fprintf(os.Stderr, "oauth: claude token stored in KV but credential-file write failed: %v\n", serr)
			}
		}
		writeJSON(w, http.StatusCreated, maskOAuthToken(t))
	case http.MethodDelete:
		if err := store.DeleteOAuthToken(d, provider); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"provider": provider, "revoked": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func oauthInitHandler(w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tpl, ok := oauthTemplates[provider]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":     "unknown provider — paste-token mode still available via POST /api/oauth/" + provider,
			"supported": keysOf(oauthTemplates),
		})
		return
	}
	var body struct {
		ClientID    string `json:"clientId"`
		RedirectURI string `json:"redirectUri"`
		Scope       string `json:"scope"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ClientID == "" {
		body.ClientID = "PLACEHOLDER_" + tpl.ClientIDEnv
	}

	if body.RedirectURI == "" {
		body.RedirectURI = oauthCallbackURL(r, provider)
	}
	if body.Scope == "" {
		body.Scope = tpl.DefaultScope
	}

	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)
	verifierBytes := make([]byte, 32)
	_, _ = rand.Read(verifierBytes)
	verifier := hex.EncodeToString(verifierBytes)

	d, _ := store.Open()
	_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
		Provider:  provider + ":pending",
		TokenType: "pkce-pending",
		Scope:     body.Scope,
		Extra: map[string]any{
			"state":       state,
			"verifier":    verifier,
			"clientId":    body.ClientID,
			"redirectUri": body.RedirectURI,
			"expiresAt":   time.Now().Add(10 * time.Minute).Format(time.RFC3339),
		},
	})

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", body.ClientID)
	q.Set("redirect_uri", body.RedirectURI)
	q.Set("scope", body.Scope)
	q.Set("state", state)

	sum := sha256.Sum256([]byte(verifier))
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("code_challenge_method", "S256")
	authURL := tpl.AuthURL + "?" + q.Encode()
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":    provider,
		"authUrl":     authURL,
		"state":       state,
		"redirectUri": body.RedirectURI,
		"note":        "Open authUrl in browser; on success the callback endpoint will store token.",
	})
}

func oauthCallbackHandler(w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	d, _ := store.Open()
	pending, _ := store.GetOAuthToken(d, provider+":pending")
	if pending == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no pending OAuth init for " + provider})
		return
	}
	extra, _ := pending.Extra.(map[string]any)
	storedState, _ := extra["state"].(string)

	if extra == nil || subtle.ConstantTimeCompare([]byte(storedState), []byte(state)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "state mismatch"})
		return
	}

	resp := map[string]any{
		"provider": provider,
		"code":     code,
		"state":    state,
		"note":     "Exchange code → token via /api/oauth/" + provider + " POST { accessToken: ... } once obtained.",
	}

	tpl := oauthTemplates[provider]
	clientID, _ := extra["clientId"].(string)
	redirectURI, _ := extra["redirectUri"].(string)
	verifier, _ := extra["verifier"].(string)
	if tpl.TokenURL != "" && clientID != "" && !strings.HasPrefix(clientID, "PLACEHOLDER_") {
		form := url.Values{}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("client_id", clientID)
		form.Set("redirect_uri", redirectURI)
		form.Set("code_verifier", verifier)
		req, _ := http.NewRequest("POST", tpl.TokenURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		exResp, err := mediaHTTPClient.Do(req)
		if err == nil {
			defer exResp.Body.Close()
			var tok struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
				TokenType    string `json:"token_type"`
				Scope        string `json:"scope"`
			}
			_ = json.NewDecoder(exResp.Body).Decode(&tok)
			if tok.AccessToken != "" {
				saved := &store.OAuthTokenRecord{
					Provider: provider, AccessToken: tok.AccessToken,
					RefreshToken: tok.RefreshToken, TokenType: tok.TokenType,
					Scope: tok.Scope,
				}
				if tok.ExpiresIn > 0 {
					saved.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Format(time.RFC3339)
				}
				_ = store.UpsertOAuthToken(d, saved)
				_ = store.DeleteOAuthToken(d, provider+":pending")
				resp["exchanged"] = true
				resp["token"] = maskOAuthToken(saved)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func maskOAuthToken(t *store.OAuthTokenRecord) map[string]any {
	mask := func(s string) string {
		if len(s) < 8 {
			return strings.Repeat("•", len(s))
		}
		return s[:4] + strings.Repeat("•", len(s)-8) + s[len(s)-4:]
	}
	return map[string]any{
		"provider":   t.Provider,
		"tokenType":  t.TokenType,
		"scope":      t.Scope,
		"expiresAt":  t.ExpiresAt,
		"updatedAt":  t.UpdatedAt,
		"hasAccess":  t.AccessToken != "",
		"hasRefresh": t.RefreshToken != "",
		"accessHint": mask(t.AccessToken),
	}
}

func keysOf(m map[string]oauthProviderTemplate) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func init() {
	_ = fmt.Sprintf
}

func oauthCallbackURL(r *http.Request, provider string) string {
	path := "/api/oauth/" + provider + "/callback"

	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("FLOW_ROUTER_PUBLIC_URL")), "/"); base != "" {
		return base + path
	}

	host := strings.TrimSpace(os.Getenv("FLOW_ROUTER_HOST"))
	if host == "" {
		if r != nil && isLoopbackHostPort(r.Host) {
			host = r.Host
		} else {
			host = "127.0.0.1:2402"
		}
	}

	scheme := strings.TrimSpace(os.Getenv("FLOW_ROUTER_SCHEME"))
	if scheme == "" {
		scheme = "https"
		if isLoopbackHostPort(host) {
			scheme = "http"
		}
		if os.Getenv("FLOW_ROUTER_TRUST_PROXY") == "1" && r != nil {
			if xf := r.Header.Get("X-Forwarded-Proto"); xf == "http" || xf == "https" {
				scheme = xf
			}
		}
	}
	return scheme + "://" + host + path
}

func isLoopbackHostPort(hostport string) bool {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return false
	}
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	h = strings.ToLower(strings.Trim(h, "[]"))
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
