// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package creds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func SaveClaude(accessToken, refreshToken, expiresAt string) error {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return os.ErrInvalid
	}
	p := credentialsPath()

	var cf CredentialsFile
	if raw, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(raw, &cf)
	}

	exp := int64(9999999999999)
	if v, err := strconv.ParseInt(strings.TrimSpace(expiresAt), 10, 64); err == nil && v > 0 {
		exp = v
	}

	cf.ClaudeAiOauth.AccessToken = accessToken
	if rt := strings.TrimSpace(refreshToken); rt != "" {
		cf.ClaudeAiOauth.RefreshToken = rt
	}
	cf.ClaudeAiOauth.ExpiresAt = exp
	if cf.ClaudeAiOauth.SubscriptionType == "" {
		cf.ClaudeAiOauth.SubscriptionType = "max"
	}

	out, err := json.MarshalIndent(&cf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		return err
	}

	InvalidateCache()
	return nil
}
