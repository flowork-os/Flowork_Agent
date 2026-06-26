// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

type OAuthTokenRecord struct {
	Provider     string `json:"provider"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	IDToken      string `json:"idToken,omitempty"`
	TokenType    string `json:"tokenType"`
	Scope        string `json:"scope,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	Extra        any    `json:"extra,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
}

const oauthKVPrefix = "oauth:"

func ListOAuthTokens(d *sql.DB) ([]OAuthTokenRecord, error) {
	ts, err := kvList[OAuthTokenRecord](d, oauthKVPrefix)
	for i := range ts {
		ts[i].AccessToken = DecryptSecret(ts[i].AccessToken)
		ts[i].RefreshToken = DecryptSecret(ts[i].RefreshToken)
		ts[i].IDToken = DecryptSecret(ts[i].IDToken)
	}
	return ts, err
}

func GetOAuthToken(d *sql.DB, provider string) (*OAuthTokenRecord, error) {
	t, err := kvGetByKey[OAuthTokenRecord](d, oauthKVPrefix+provider)
	if t != nil {
		t.AccessToken = DecryptSecret(t.AccessToken)
		t.RefreshToken = DecryptSecret(t.RefreshToken)
		t.IDToken = DecryptSecret(t.IDToken)
	}
	return t, err
}

func UpsertOAuthToken(d *sql.DB, t *OAuthTokenRecord) error {
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	rec := *t
	rec.AccessToken = EncryptSecret(rec.AccessToken)
	rec.RefreshToken = EncryptSecret(rec.RefreshToken)
	rec.IDToken = EncryptSecret(rec.IDToken)
	return kvUpsert(d, oauthKVPrefix+t.Provider, &rec)
}

func DeleteOAuthToken(d *sql.DB, provider string) error {
	return kvDelete(d, oauthKVPrefix+provider)
}

type MCPServer struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Enabled   bool              `json:"enabled"`
	UpdatedAt string            `json:"updatedAt"`
}

const mcpKVPrefix = "mcp:"

func ListMCPServers(d *sql.DB) ([]MCPServer, error) {
	return kvList[MCPServer](d, mcpKVPrefix)
}

func GetMCPServer(d *sql.DB, id string) (*MCPServer, error) {
	return kvGetByKey[MCPServer](d, mcpKVPrefix+id)
}

func UpsertMCPServer(d *sql.DB, m *MCPServer) error {
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return kvUpsert(d, mcpKVPrefix+m.ID, m)
}

func DeleteMCPServer(d *sql.DB, id string) error {
	return kvDelete(d, mcpKVPrefix+id)
}

type TunnelState struct {
	CloudflareEnabled  bool   `json:"cloudflareEnabled"`
	CloudflareURL      string `json:"cloudflareUrl,omitempty"`
	CloudflareToken    string `json:"cloudflareToken,omitempty"`
	CloudflarePID      int    `json:"cloudflarePid,omitempty"`
	TailscaleInstalled bool   `json:"tailscaleInstalled"`
	TailscaleEnabled   bool   `json:"tailscaleEnabled"`
	TailscaleURL       string `json:"tailscaleUrl,omitempty"`
	DashboardAccess    bool   `json:"dashboardAccess"`
	UpdatedAt          string `json:"updatedAt"`
}

const tunnelKey = "tunnel:state"

func LoadTunnelState(d *sql.DB) (*TunnelState, error) {
	t, err := kvGetByKey[TunnelState](d, tunnelKey)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return &TunnelState{}, nil
	}
	return t, nil
}

func SaveTunnelState(d *sql.DB, t *TunnelState) error {
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return kvUpsert(d, tunnelKey, t)
}

type LocalePref struct {
	Locale    string `json:"locale"`
	Timezone  string `json:"timezone"`
	Theme     string `json:"theme"`
	UpdatedAt string `json:"updatedAt"`
}

const localeKey = "locale:pref"

func LoadLocalePref(d *sql.DB) (*LocalePref, error) {
	p, err := kvGetByKey[LocalePref](d, localeKey)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return &LocalePref{Locale: "id", Timezone: "Asia/Jakarta", Theme: "dark"}, nil
	}
	return p, nil
}

func SaveLocalePref(d *sql.DB, p *LocalePref) error {
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return kvUpsert(d, localeKey, p)
}

type CLIToolState struct {
	ToolID          string         `json:"toolId"`
	Installed       bool           `json:"installed"`
	HasCredentials  bool           `json:"hasCredentials"`
	BinaryPath      string         `json:"binaryPath,omitempty"`
	CredentialsPath string         `json:"credentialsPath,omitempty"`
	Version         string         `json:"version,omitempty"`
	Settings        map[string]any `json:"settings,omitempty"`
	Status          string         `json:"status"`
	Notes           string         `json:"notes,omitempty"`
	UpdatedAt       string         `json:"updatedAt"`
}

const cliToolKVPrefix = "clitool:"

func ListCLIToolState(d *sql.DB) ([]CLIToolState, error) {
	return kvList[CLIToolState](d, cliToolKVPrefix)
}

func GetCLIToolState(d *sql.DB, toolID string) (*CLIToolState, error) {
	return kvGetByKey[CLIToolState](d, cliToolKVPrefix+toolID)
}

func UpsertCLIToolState(d *sql.DB, s *CLIToolState) error {
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return kvUpsert(d, cliToolKVPrefix+s.ToolID, s)
}

func kvList[T any](d *sql.DB, prefix string) ([]T, error) {
	rows, err := d.Query(`SELECT k, v FROM kv WHERE k LIKE ? ORDER BY k ASC`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		var t T
		if err := json.Unmarshal([]byte(v), &t); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func kvGetByKey[T any](d *sql.DB, key string) (*T, error) {
	row := d.QueryRow(`SELECT v FROM kv WHERE k = ?`, key)
	var v string
	err := row.Scan(&v)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t T
	if err := json.Unmarshal([]byte(v), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func kvUpsert(d *sql.DB, key string, val any) error {
	v, err := json.Marshal(val)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, ?)
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
		key, string(v), time.Now().UTC().Format(time.RFC3339))
	return err
}

func kvDelete(d *sql.DB, key string) error {
	_, err := d.Exec(`DELETE FROM kv WHERE k = ?`, key)
	return err
}
