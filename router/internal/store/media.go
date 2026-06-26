// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MediaCategoryEmbedding   = "embedding"
	MediaCategoryTextToImage = "text-to-image"
	MediaCategoryTTS         = "tts"
	MediaCategorySTT         = "stt"
	MediaCategoryWebFetch    = "web-fetch-search"
)

type MediaProvider struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	BaseURL   string    `json:"baseUrl"`
	APIKey    string    `json:"apiKey,omitempty"`
	Models    []string  `json:"models"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

const mediaKVPrefix = "media:"

func ListMediaProviders(d *sql.DB, category string) ([]MediaProvider, error) {
	prefix := mediaKVPrefix
	if category != "" {
		prefix += category + ":"
	}
	rows, err := d.Query(`SELECT k, v FROM kv WHERE k LIKE ? ORDER BY k ASC`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaProvider
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		var m MediaProvider
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			continue
		}
		m.APIKey = DecryptSecret(m.APIKey)
		out = append(out, m)
	}
	return out, nil
}

func UpsertMediaProvider(d *sql.DB, m *MediaProvider) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
		m.CreatedAt = time.Now().UTC()
	}
	if m.Category == "" {
		return fmt.Errorf("media: category required")
	}

	persist := *m
	persist.APIKey = EncryptSecret(persist.APIKey)
	v, _ := json.Marshal(&persist)
	_, err := d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, ?)
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
		mediaKVPrefix+m.Category+":"+m.ID, string(v), time.Now().UTC().Format(time.RFC3339))
	return err
}

func DeleteMediaProvider(d *sql.DB, category, id string) error {
	_, err := d.Exec(`DELETE FROM kv WHERE k = ?`, mediaKVPrefix+category+":"+id)
	return err
}
