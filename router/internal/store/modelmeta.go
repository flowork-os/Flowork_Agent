// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type ModelAlias struct {
	Alias      string    `json:"alias"`
	ProviderID string    `json:"providerId"`
	Model      string    `json:"model"`
	CreatedAt  time.Time `json:"createdAt"`
}

func ListModelAliases(d *sql.DB) ([]ModelAlias, error) {
	rows, err := d.Query(`SELECT alias, providerId, model, createdAt FROM modelAlias ORDER BY alias ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelAlias
	for rows.Next() {
		var a ModelAlias
		var ts string
		if err := rows.Scan(&a.Alias, &a.ProviderID, &a.Model, &ts); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, a)
	}
	return out, nil
}

func UpsertModelAlias(d *sql.DB, a *ModelAlias) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := d.Exec(`INSERT INTO modelAlias (alias, providerId, model, createdAt)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET providerId=excluded.providerId, model=excluded.model`,
		a.Alias, a.ProviderID, a.Model, a.CreatedAt.Format(time.RFC3339))
	return err
}

func DeleteModelAlias(d *sql.DB, alias string) error {
	_, err := d.Exec(`DELETE FROM modelAlias WHERE alias = ?`, alias)
	return err
}

type ModelAvailability struct {
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	LatencyMs    int       `json:"latencyMs"`
	CheckedAt    time.Time `json:"checkedAt"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
}

func ListModelAvailability(d *sql.DB) ([]ModelAvailability, error) {
	rows, err := d.Query(`SELECT provider, model, status, latencyMs, checkedAt, COALESCE(errorMessage, '') FROM modelAvailability ORDER BY provider, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelAvailability
	for rows.Next() {
		var a ModelAvailability
		var ts string
		if err := rows.Scan(&a.Provider, &a.Model, &a.Status, &a.LatencyMs, &ts, &a.ErrorMessage); err != nil {
			return nil, err
		}
		a.CheckedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, a)
	}
	return out, nil
}

func RecordAvailability(d *sql.DB, a *ModelAvailability) error {
	a.CheckedAt = time.Now().UTC()
	_, err := d.Exec(`INSERT INTO modelAvailability (provider, model, status, latencyMs, checkedAt, errorMessage)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider, model) DO UPDATE SET
			status=excluded.status,
			latencyMs=excluded.latencyMs,
			checkedAt=excluded.checkedAt,
			errorMessage=excluded.errorMessage`,
		a.Provider, a.Model, a.Status, a.LatencyMs, a.CheckedAt.Format(time.RFC3339), a.ErrorMessage)
	return err
}

type ModelCustom struct {
	ID                string    `json:"id"`
	ProviderID        string    `json:"providerId"`
	Model             string    `json:"model"`
	DisplayName       string    `json:"displayName"`
	ContextWindow     int       `json:"contextWindow"`
	MaxOutputTokens   int       `json:"maxOutputTokens"`
	SupportsTools     bool      `json:"supportsTools"`
	SupportsVision    bool      `json:"supportsVision"`
	SupportsStreaming bool      `json:"supportsStreaming"`
	CreatedAt         time.Time `json:"createdAt"`
}

func ListCustomModels(d *sql.DB) ([]ModelCustom, error) {
	rows, err := d.Query(`SELECT id, COALESCE(providerId, ''), model, COALESCE(displayName, ''), contextWindow, maxOutputTokens, supportsTools, supportsVision, supportsStreaming, createdAt FROM modelsCustom ORDER BY model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelCustom
	for rows.Next() {
		var m ModelCustom
		var ts string
		var st, sv, ss int
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.Model, &m.DisplayName, &m.ContextWindow, &m.MaxOutputTokens, &st, &sv, &ss, &ts); err != nil {
			return nil, err
		}
		m.SupportsTools = st != 0
		m.SupportsVision = sv != 0
		m.SupportsStreaming = ss != 0
		m.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, m)
	}
	return out, nil
}

func UpsertCustomModel(d *sql.DB, m *ModelCustom) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
		m.CreatedAt = time.Now().UTC()
	}
	st, sv, ss := 0, 0, 0
	if m.SupportsTools {
		st = 1
	}
	if m.SupportsVision {
		sv = 1
	}
	if m.SupportsStreaming {
		ss = 1
	}
	_, err := d.Exec(`INSERT INTO modelsCustom (id, providerId, model, displayName, contextWindow, maxOutputTokens, supportsTools, supportsVision, supportsStreaming, createdAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			providerId=excluded.providerId,
			model=excluded.model,
			displayName=excluded.displayName,
			contextWindow=excluded.contextWindow,
			maxOutputTokens=excluded.maxOutputTokens,
			supportsTools=excluded.supportsTools,
			supportsVision=excluded.supportsVision,
			supportsStreaming=excluded.supportsStreaming`,
		m.ID, m.ProviderID, m.Model, m.DisplayName, m.ContextWindow, m.MaxOutputTokens, st, sv, ss, m.CreatedAt.Format(time.RFC3339))
	return err
}

func DeleteCustomModel(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM modelsCustom WHERE id = ?`, id)
	return err
}

type ModelDisabled struct {
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	DisabledAt time.Time `json:"disabledAt"`
	Reason     string    `json:"reason"`
}

func ListDisabledModels(d *sql.DB) ([]ModelDisabled, error) {
	rows, err := d.Query(`SELECT provider, model, disabledAt, COALESCE(reason, '') FROM modelsDisabled ORDER BY provider, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelDisabled
	for rows.Next() {
		var m ModelDisabled
		var ts string
		if err := rows.Scan(&m.Provider, &m.Model, &ts, &m.Reason); err != nil {
			return nil, err
		}
		m.DisabledAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, m)
	}
	return out, nil
}

func DisableModel(d *sql.DB, provider, model, reason string) error {
	_, err := d.Exec(`INSERT INTO modelsDisabled (provider, model, disabledAt, reason)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider, model) DO UPDATE SET disabledAt=excluded.disabledAt, reason=excluded.reason`,
		provider, model, time.Now().UTC().Format(time.RFC3339), reason)
	return err
}

func EnableModel(d *sql.DB, provider, model string) error {
	_, err := d.Exec(`DELETE FROM modelsDisabled WHERE provider = ? AND model = ?`, provider, model)
	return err
}

func IsModelDisabled(d *sql.DB, provider, model string) bool {
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM modelsDisabled WHERE provider = ? AND model = ?`, provider, model).Scan(&n)
	return n > 0
}
