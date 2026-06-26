// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
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
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func authStatusHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	settings, _ := store.LoadSettings(d)
	requireLogin := false
	authMode := "none"
	if settings != nil {
		requireLogin = settings.RequireLogin
		authMode = settings.AuthMode
	}
	token := extractAuthToken(r)
	if token == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"requireLogin":  requireLogin,
			"authMode":      authMode,
		})
		return
	}
	s, err := store.GetSessionByToken(d, token)
	if err != nil || s == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"requireLogin":  requireLogin,
			"authMode":      authMode,
		})
		return
	}
	_ = store.TouchSession(d, s.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"requireLogin":  requireLogin,
		"authMode":      authMode,
		"session": map[string]any{
			"userId":    s.UserID,
			"createdAt": s.CreatedAt.Format(time.RFC3339),
			"expiresAt": s.ExpiresAt.Format(time.RFC3339),
		},
	})
}

func authLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIPForLock(r)
	if locked, retry := loginCheckLock(ip); locked {
		w.Header().Set("Retry-After", strconvItoa(retry))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":      "too many failed attempts",
			"retryAfter": retry,
		})
		return
	}
	d, _ := store.Open()
	settings, err := store.LoadSettings(d)
	if err != nil {
		http.Error(w, "settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if settings.AuthMode != "password" {
		http.Error(w, "auth mode != password", http.StatusBadRequest)
		return
	}
	var body struct {
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !verifyPassword(settings.Password, body.Password) {
		locked, retry := loginRecordFail(ip)
		if locked {
			w.Header().Set("Retry-After", strconvItoa(retry))
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error":      "too many failed attempts",
				"retryAfter": retry,
			})
			return
		}
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	loginRecordSuccess(ip)
	userID := body.Username
	if userID == "" {
		userID = "admin"
	}
	s, err := store.CreateSession(d, userID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		http.Error(w, "session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(),
		Expires:  s.ExpiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":     s.Token,
		"expiresAt": s.ExpiresAt.Format(time.RFC3339),
	})
}

func authLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := extractAuthToken(r)
	if token != "" {
		d, _ := store.Open()
		_ = store.DeleteSession(d, token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"loggedOut": true})
}

func authOIDCHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	settings, _ := store.LoadSettings(d)
	if settings == nil || settings.AuthMode != "oidc" {
		http.Error(w, "OIDC not configured (set authMode=oidc + oidcConfig)", http.StatusBadRequest)
		return
	}
	issuer, clientID, _, redirectURI, scopes := oidcConfigFromSettings(settings)
	writeJSON(w, http.StatusOK, map[string]any{
		"authMode":    "oidc",
		"issuer":      issuer,
		"clientId":    clientID,
		"redirectUri": redirectURI,
		"scopes":      scopes,
		"configured":  issuer != "" && clientID != "",
		"startUrl":    "/api/auth/oidc/init",
	})
}

const sessionCookieName = "flow_router_session"

func extractAuthToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func hashPassword(plain string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	const (
		t      = 1
		m      = 64 * 1024
		p      = 4
		keyLen = 32
	)
	h := argon2.IDKey([]byte(plain), salt, t, m, p, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, m, t, p,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(h))
}

func hashPasswordSHA(plain string) string {
	salt := "flow_router_local_v1"
	h := sha256.Sum256([]byte(salt + ":" + plain))
	return hex.EncodeToString(h[:])
}

func verifyPassword(stored, plain string) bool {
	if stored == "" || plain == "" {
		return false
	}
	if strings.HasPrefix(stored, "$argon2id$") {
		return verifyArgon2(stored, plain)
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(hashPasswordSHA(plain))) == 1
}

func verifyArgon2(stored, plain string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 {
		return false
	}
	var version, m, t, p int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(plain), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func cookieSecure() bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(os.Getenv("FLOW_ROUTER_PUBLIC_URL"))), "https") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("FLOW_ROUTER_SCHEME")), "https") {
		return true
	}
	return os.Getenv("FLOW_ROUTER_TRUST_PROXY") == "1"
}
