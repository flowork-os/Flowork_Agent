// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package creds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CredentialsFile struct {
	ClaudeAiOauth struct {
		AccessToken      string   `json:"accessToken"`
		RefreshToken     string   `json:"refreshToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
		RateLimitTier    string   `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
	OrganizationUUID string `json:"organizationUuid"`
}

var (
	cachedMu       sync.Mutex
	cachedCreds    *CredentialsFile
	cachedLoadedAt time.Time
	cacheValidity  = 30 * time.Second
)

func credentialsPath() string {
	if p := os.Getenv("FLOW_CREDS_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", ".credentials.json")
}

func Load() (*CredentialsFile, error) {
	cachedMu.Lock()
	defer cachedMu.Unlock()

	if cachedCreds != nil && time.Since(cachedLoadedAt) < cacheValidity {
		return cachedCreds, nil
	}

	p := credentialsPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("creds: read %s: %w", p, err)
	}

	var c CredentialsFile
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("creds: parse %s: %w", p, err)
	}

	if c.ClaudeAiOauth.AccessToken == "" {
		return nil, fmt.Errorf("creds: accessToken empty di %s — login Claude Code dulu", p)
	}

	cachedCreds = &c
	cachedLoadedAt = time.Now()
	return cachedCreds, nil
}

func (c *CredentialsFile) IsExpired() bool {
	if c.ClaudeAiOauth.ExpiresAt == 0 {
		return false
	}
	expiry := time.UnixMilli(c.ClaudeAiOauth.ExpiresAt)
	return time.Now().Add(60 * time.Second).After(expiry)
}

func (c *CredentialsFile) MaskedAccessToken() string {
	t := c.ClaudeAiOauth.AccessToken
	if len(t) < 20 {
		return "[masked]"
	}
	return t[:10] + "...[masked total " + fmt.Sprintf("%d", len(t)) + " chars]"
}

func InvalidateCache() {
	cachedMu.Lock()
	defer cachedMu.Unlock()
	cachedCreds = nil
	cachedLoadedAt = time.Time{}
}
