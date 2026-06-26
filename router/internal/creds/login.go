// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package creds

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"os"
	"strings"
)

func claudeAuthURL() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_AUTH_URL")); v != "" {
		return v
	}
	return "https://claude.ai/oauth/authorize"
}

func claudeRedirectURI() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_REDIRECT_URI")); v != "" {
		return v
	}
	return "https://console.anthropic.com/oauth/code/callback"
}

func claudeScopes() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_SCOPES")); v != "" {
		return v
	}
	return "org:create_api_key user:profile user:inference"
}

func PKCEPair() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func RandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func ClaudeAuthorizeURL(challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", claudeClientID())
	q.Set("redirect_uri", claudeRedirectURI())
	q.Set("scope", claudeScopes())
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	return claudeAuthURL() + "?" + q.Encode()
}

func ExchangeClaudeCode(code, verifier, state string) (access, refresh string, expiresAtMs int64, err error) {
	return postClaudeToken(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"state":         state,
		"client_id":     claudeClientID(),
		"redirect_uri":  claudeRedirectURI(),
		"code_verifier": verifier,
	})
}
