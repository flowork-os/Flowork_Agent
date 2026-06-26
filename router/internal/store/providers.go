// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	AuthTypeAPIKey       = "api_key"
	AuthTypeSubscription = "subscription"
	AuthTypeNone         = "none"
)

type ProviderConnection struct {
	ID        string         `json:"id"`
	Provider  string         `json:"provider"`
	AuthType  string         `json:"authType"`
	Name      string         `json:"name"`
	Email     string         `json:"email,omitempty"`
	Priority  int            `json:"priority"`
	IsActive  bool           `json:"isActive"`
	Data      map[string]any `json:"data"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

const (
	CfgBaseURL     = "baseUrl"
	CfgAPIKey      = "apiKey"
	CfgModels      = "models"
	CfgFormat      = "format"
	CfgHeaders     = "headers"
	CfgTokenSource = "tokenSource"
)

func ListProviders(d *sql.DB) ([]ProviderConnection, error) {
	rows, err := d.Query(`SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt
		FROM providerConnections ORDER BY priority ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query providers: %w", err)
	}
	defer rows.Close()

	var out []ProviderConnection
	for rows.Next() {
		var p ProviderConnection
		var isActive int
		var dataJSON, createdStr, updatedStr string
		var email sql.NullString
		if err := rows.Scan(&p.ID, &p.Provider, &p.AuthType, &p.Name, &email,
			&p.Priority, &isActive, &dataJSON, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if email.Valid {
			p.Email = email.String
		}
		p.IsActive = isActive == 1
		_ = json.Unmarshal([]byte(dataJSON), &p.Data)
		decryptProviderKey(p.Data)
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		out = append(out, p)
	}
	return out, nil
}

func GetProvider(d *sql.DB, id string) (*ProviderConnection, error) {
	row := d.QueryRow(`SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt
		FROM providerConnections WHERE id = ?`, id)
	var p ProviderConnection
	var isActive int
	var dataJSON, createdStr, updatedStr string
	var email sql.NullString
	if err := row.Scan(&p.ID, &p.Provider, &p.AuthType, &p.Name, &email,
		&p.Priority, &isActive, &dataJSON, &createdStr, &updatedStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if email.Valid {
		p.Email = email.String
	}
	p.IsActive = isActive == 1
	_ = json.Unmarshal([]byte(dataJSON), &p.Data)
	decryptProviderKey(p.Data)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &p, nil
}

func decryptProviderKey(data map[string]any) {
	if data == nil {
		return
	}
	if k, ok := data[CfgAPIKey].(string); ok && k != "" {
		data[CfgAPIKey] = DecryptSecret(k)
	}
}

func encryptProviderData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	k, ok := data[CfgAPIKey].(string)
	if !ok || k == "" {
		return data
	}
	cp := make(map[string]any, len(data))
	for key, v := range data {
		cp[key] = v
	}
	cp[CfgAPIKey] = EncryptSecret(k)
	return cp
}

func FindActiveByModel(d *sql.DB, model string) ([]ProviderConnection, error) {
	all, err := ListProviders(d)
	if err != nil {
		return nil, err
	}
	var match []ProviderConnection
	model = strings.TrimSpace(model)
	modelLower := strings.ToLower(model)
	for _, p := range all {
		if !p.IsActive {
			continue
		}
		models, _ := p.Data[CfgModels].([]any)
		for _, m := range models {
			ms, ok := m.(string)
			if !ok {
				continue
			}
			msLower := strings.ToLower(ms)
			if ms == "*" || msLower == modelLower {
				match = append(match, p)
				break
			}

			if strings.HasSuffix(ms, "*") {
				prefix := strings.TrimSuffix(msLower, "*")
				if strings.HasPrefix(modelLower, prefix) {
					match = append(match, p)
					break
				}
			}
		}
	}
	return match, nil
}

func UpsertProvider(d *sql.DB, p *ProviderConnection) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()

	persistData := p.Data
	if p.ID != "" && p.Data != nil {
		if k, ok := p.Data[CfgAPIKey].(string); !ok || k == "" || strings.ContainsRune(k, '•') {
			if existing, _ := GetProvider(d, p.ID); existing != nil {
				if ek, ok := existing.Data[CfgAPIKey].(string); ok && ek != "" {
					persistData = make(map[string]any, len(p.Data)+1)
					for kk, vv := range p.Data {
						persistData[kk] = vv
					}
					persistData[CfgAPIKey] = ek
				}
			}
		}
	}
	dataJSON, _ := json.Marshal(encryptProviderData(persistData))
	active := 0
	if p.IsActive {
		active = 1
	}
	createdStr := p.CreatedAt.Format(time.RFC3339)
	if createdStr == "0001-01-01T00:00:00Z" {
		createdStr = now
	}

	_, err := d.Exec(`INSERT INTO providerConnections (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider=excluded.provider, authType=excluded.authType, name=excluded.name,
			email=excluded.email, priority=excluded.priority, isActive=excluded.isActive,
			data=excluded.data, updatedAt=excluded.updatedAt`,
		p.ID, p.Provider, p.AuthType, p.Name, p.Email, p.Priority, active, string(dataJSON),
		createdStr, p.UpdatedAt.Format(time.RFC3339))
	return err
}

func DeleteProvider(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM providerConnections WHERE id = ?`, id)
	return err
}

func AugmentTierTags(d *sql.DB) error {
	providers, err := ListProviders(d)
	if err != nil {
		return err
	}
	for _, p := range providers {
		tags, _ := p.Data["tags"].([]any)
		hasTier := false
		for _, t := range tags {
			if s, ok := t.(string); ok && strings.HasPrefix(s, "tier:") {
				hasTier = true
				break
			}
		}
		if hasTier {
			continue
		}
		var inferred []string
		switch p.Provider {
		case "local-llama", "ollama":
			inferred = []string{"tier:cheap"}
		case "anthropic":
			inferred = []string{"tier:standard", "tier:strong"}
		case "openai":
			inferred = []string{"tier:standard"}
		case "google", "gemini":
			inferred = []string{"tier:standard"}
		default:
			continue
		}
		for _, t := range inferred {
			tags = append(tags, t)
		}
		if p.Data == nil {
			p.Data = map[string]any{}
		}
		p.Data["tags"] = tags
		if err := UpsertProvider(d, &p); err != nil {
			return fmt.Errorf("augment tier tags for %s: %w", p.ID, err)
		}
	}
	return nil
}

func SeedDefaults(d *sql.DB) error {
	existing, err := ListProviders(d)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}

	claude := &ProviderConnection{
		Provider: "anthropic",
		AuthType: AuthTypeSubscription,
		Name:     "Claude Pro/Max Subscription",
		Priority: 10,
		IsActive: true,
		Data: map[string]any{
			CfgBaseURL:     "https://api.anthropic.com/v1",
			CfgFormat:      "anthropic",
			CfgTokenSource: "claude_credentials",
			CfgModels: []any{
				"claude-fable-5", "claude-opus-4-8", "claude-opus-4-7",
				"claude-sonnet-4-6", "claude-haiku-4-5",
				"claude-*",
			},
			"tags": []any{"tier:standard", "tier:strong"},
		},
	}
	if err := UpsertProvider(d, claude); err != nil {
		return fmt.Errorf("seed claude: %w", err)
	}

	local := &ProviderConnection{
		Provider: "local-llama",
		AuthType: AuthTypeNone,
		Name:     "Local llama-server",
		Priority: 100,
		IsActive: false,
		Data: map[string]any{
			CfgBaseURL: "http://127.0.0.1:8080/v1",
			CfgFormat:  "openai",
			CfgModels: []any{
				"brain-flowork", "brain-flowork.gguf",
				"qwen3-8b", "qwen*",
				"local-*", "mrflow",
				"*",
			},
			"tags": []any{"tier:cheap", "local"},
		},
	}
	if err := UpsertProvider(d, local); err != nil {
		return fmt.Errorf("seed local: %w", err)
	}

	const keyPlaceholder = "PASTE_YOUR_KEY_HERE"
	catalog := []*ProviderConnection{
		{Provider: "openai", AuthType: AuthTypeAPIKey, Name: "OpenAI", Priority: 20, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.openai.com/v1", CfgFormat: "openai",
				CfgModels: []any{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-*"}, "tags": []any{"tier:standard", "tier:strong"}}},
		{Provider: "anthropic-apikey", AuthType: AuthTypeAPIKey, Name: "Anthropic API (key)", Priority: 21, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.anthropic.com/v1", CfgFormat: "anthropic",
				CfgModels: []any{"claude-fable-5", "claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5", "claude-*"}, "tags": []any{"tier:standard", "tier:strong"}}},
		{Provider: "gemini", AuthType: AuthTypeAPIKey, Name: "Google Gemini (API key)", Priority: 22, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://generativelanguage.googleapis.com/v1beta", CfgFormat: "gemini",
				CfgModels: []any{"gemini-3.1-pro-preview", "gemini-3.5-flash", "gemini-3.1-flash-lite", "gemini-*"}, "tags": []any{"tier:standard", "tier:strong"}}},
		{Provider: "deepseek", AuthType: AuthTypeAPIKey, Name: "DeepSeek", Priority: 23, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.deepseek.com/v1", CfgFormat: "openai",
				CfgModels: []any{"deepseek-chat", "deepseek-reasoner", "deepseek-*"}, "tags": []any{"tier:cheap", "tier:standard"}}},
		{Provider: "groq", AuthType: AuthTypeAPIKey, Name: "Groq", Priority: 24, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.groq.com/openai/v1", CfgFormat: "openai",
				CfgModels: []any{"llama-3.3-70b-versatile", "qwen-*", "llama-*"}, "tags": []any{"tier:cheap"}}},
		{Provider: "openrouter", AuthType: AuthTypeAPIKey, Name: "OpenRouter", Priority: 25, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://openrouter.ai/api/v1", CfgFormat: "openai",
				CfgModels: []any{"*"}, "tags": []any{"tier:standard"}}},
		{Provider: "mistral", AuthType: AuthTypeAPIKey, Name: "Mistral AI", Priority: 26, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.mistral.ai/v1", CfgFormat: "openai",
				CfgModels: []any{"mistral-large-latest", "mistral-*"}, "tags": []any{"tier:standard"}}},
		{Provider: "xai", AuthType: AuthTypeAPIKey, Name: "xAI Grok", Priority: 27, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.x.ai/v1", CfgFormat: "openai",
				CfgModels: []any{"grok-*"}, "tags": []any{"tier:standard"}}},
		{Provider: "together", AuthType: AuthTypeAPIKey, Name: "Together AI", Priority: 28, IsActive: false,
			Data: map[string]any{CfgAPIKey: keyPlaceholder, CfgBaseURL: "https://api.together.xyz/v1", CfgFormat: "openai",
				CfgModels: []any{"*"}, "tags": []any{"tier:standard"}}},
	}
	for _, c := range catalog {
		if err := UpsertProvider(d, c); err != nil {
			return fmt.Errorf("seed catalog %s: %w", c.Provider, err)
		}
	}

	return nil
}
