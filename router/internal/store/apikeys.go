// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	KeyHash          string    `json:"-"`
	KeyPrefix        string    `json:"keyPrefix"`
	PlaintextKey     string    `json:"plaintextKey,omitempty"`
	AllowedProviders string    `json:"allowedProviders"`
	DailyCapUsd      float64   `json:"dailyCapUsd"`
	MonthlyCapUsd    float64   `json:"monthlyCapUsd"`
	IsActive         bool      `json:"isActive"`
	CreatedAt        time.Time `json:"createdAt"`
	LastUsedAt       string    `json:"lastUsedAt,omitempty"`
}

func GenerateAPIKey(d *sql.DB, name, allowedProviders string, dailyCap, monthlyCap float64) (*APIKey, error) {

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("rand: %w", err)
	}
	plaintext := "flr_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(plaintext))
	hashStr := hex.EncodeToString(hash[:])

	k := &APIKey{
		ID:               uuid.NewString(),
		Name:             name,
		KeyHash:          hashStr,
		KeyPrefix:        plaintext[:14] + "...",
		PlaintextKey:     plaintext,
		AllowedProviders: allowedProviders,
		DailyCapUsd:      dailyCap,
		MonthlyCapUsd:    monthlyCap,
		IsActive:         true,
		CreatedAt:        time.Now().UTC(),
	}
	if k.AllowedProviders == "" {
		k.AllowedProviders = "*"
	}

	_, err := d.Exec(`INSERT INTO apiKeys (id, name, keyHash, keyPrefix, allowedProviders, dailyCapUsd, monthlyCapUsd, isActive, createdAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		k.ID, k.Name, k.KeyHash, k.KeyPrefix, k.AllowedProviders, k.DailyCapUsd, k.MonthlyCapUsd, k.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}
	return k, nil
}

func ListAPIKeys(d *sql.DB) ([]APIKey, error) {
	rows, err := d.Query(`SELECT id, name, keyPrefix, allowedProviders, dailyCapUsd, monthlyCapUsd, isActive, createdAt,
		COALESCE(lastUsedAt, '') FROM apiKeys ORDER BY createdAt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		var active int
		var createdStr string
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.AllowedProviders,
			&k.DailyCapUsd, &k.MonthlyCapUsd, &active, &createdStr, &k.LastUsedAt); err != nil {
			return nil, err
		}
		k.IsActive = active == 1
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		out = append(out, k)
	}
	return out, nil
}

func DeleteAPIKey(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM apiKeys WHERE id = ?`, id)
	return err
}

func SpendSince(d *sql.DB, apiKeyID, sinceDay string) (float64, error) {
	var total float64
	err := d.QueryRow(`SELECT COALESCE(SUM(costUsd), 0) FROM usageDaily
		WHERE apiKeyId = ? AND day >= ?`, apiKeyID, sinceDay).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func TotalSpendSince(d *sql.DB, sinceDay string) (float64, error) {
	var total float64
	err := d.QueryRow(`SELECT COALESCE(SUM(costUsd), 0) FROM usageDaily
		WHERE day >= ?`, sinceDay).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func VerifyAPIKey(d *sql.DB, plaintext string) (*APIKey, error) {
	hash := sha256.Sum256([]byte(plaintext))
	hashStr := hex.EncodeToString(hash[:])
	row := d.QueryRow(`SELECT id, name, keyPrefix, allowedProviders, dailyCapUsd, monthlyCapUsd, isActive, createdAt
		FROM apiKeys WHERE keyHash = ? AND isActive = 1`, hashStr)
	var k APIKey
	var active int
	var createdStr string
	if err := row.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.AllowedProviders,
		&k.DailyCapUsd, &k.MonthlyCapUsd, &active, &createdStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	k.IsActive = active == 1
	k.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

	go func() {
		_, _ = d.Exec(`UPDATE apiKeys SET lastUsedAt = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), k.ID)
	}()

	return &k, nil
}
