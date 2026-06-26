// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"time"
)

type Pricing struct {
	Provider           string    `json:"provider"`
	Model              string    `json:"model"`
	InputUsdPer1M      float64   `json:"inputUsdPer1M"`
	OutputUsdPer1M     float64   `json:"outputUsdPer1M"`
	CacheReadUsdPer1M  float64   `json:"cacheReadUsdPer1M"`
	CacheWriteUsdPer1M float64   `json:"cacheWriteUsdPer1M"`
	Currency           string    `json:"currency"`
	Source             string    `json:"source"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func ListPricing(d *sql.DB, provider string) ([]Pricing, error) {
	q := `SELECT provider, model, inputUsdPer1M, outputUsdPer1M, cacheReadUsdPer1M, cacheWriteUsdPer1M, currency, source, updatedAt FROM pricing`
	args := []any{}
	if provider != "" {
		q += ` WHERE provider = ?`
		args = append(args, provider)
	}
	q += ` ORDER BY provider, model`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pricing
	for rows.Next() {
		var p Pricing
		var ts string
		if err := rows.Scan(&p.Provider, &p.Model, &p.InputUsdPer1M, &p.OutputUsdPer1M, &p.CacheReadUsdPer1M, &p.CacheWriteUsdPer1M, &p.Currency, &p.Source, &ts); err != nil {
			return nil, err
		}
		p.UpdatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, p)
	}
	return out, nil
}

func GetPricing(d *sql.DB, provider, model string) (*Pricing, error) {
	row := d.QueryRow(`SELECT provider, model, inputUsdPer1M, outputUsdPer1M, cacheReadUsdPer1M, cacheWriteUsdPer1M, currency, source, updatedAt FROM pricing WHERE provider = ? AND model = ?`, provider, model)
	var p Pricing
	var ts string
	err := row.Scan(&p.Provider, &p.Model, &p.InputUsdPer1M, &p.OutputUsdPer1M, &p.CacheReadUsdPer1M, &p.CacheWriteUsdPer1M, &p.Currency, &p.Source, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, _ = time.Parse(time.RFC3339, ts)
	return &p, nil
}

func LookupPricingByModel(d *sql.DB, model string) (*Pricing, error) {
	if model == "" {
		return nil, nil
	}
	scan := func(row *sql.Row) (*Pricing, error) {
		var p Pricing
		var ts string
		err := row.Scan(&p.Provider, &p.Model, &p.InputUsdPer1M, &p.OutputUsdPer1M, &p.CacheReadUsdPer1M, &p.CacheWriteUsdPer1M, &p.Currency, &p.Source, &ts)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		p.UpdatedAt, _ = time.Parse(time.RFC3339, ts)
		return &p, nil
	}
	const cols = `provider, model, inputUsdPer1M, outputUsdPer1M, cacheReadUsdPer1M, cacheWriteUsdPer1M, currency, source, updatedAt`

	if p, err := scan(d.QueryRow(`SELECT `+cols+` FROM pricing WHERE model = ? LIMIT 1`, model)); err != nil || p != nil {
		return p, err
	}

	if i := len(model) - 1; i > 0 {
		if slash := indexByteRev(model, '/'); slash >= 0 && slash < len(model)-1 {
			bare := model[slash+1:]
			if p, err := scan(d.QueryRow(`SELECT `+cols+` FROM pricing WHERE model = ? LIMIT 1`, bare)); err != nil || p != nil {
				return p, err
			}
		}
	}

	pref := model
	if len(pref) > 10 {
		pref = pref[:10]
	}
	return scan(d.QueryRow(`SELECT `+cols+` FROM pricing WHERE model LIKE ? LIMIT 1`, pref+"%"))
}

func indexByteRev(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func UpsertPricing(d *sql.DB, p *Pricing) error {
	if p.Currency == "" {
		p.Currency = "USD"
	}
	p.UpdatedAt = time.Now().UTC()
	_, err := d.Exec(`INSERT INTO pricing (provider, model, inputUsdPer1M, outputUsdPer1M, cacheReadUsdPer1M, cacheWriteUsdPer1M, currency, source, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider, model) DO UPDATE SET
			inputUsdPer1M=excluded.inputUsdPer1M,
			outputUsdPer1M=excluded.outputUsdPer1M,
			cacheReadUsdPer1M=excluded.cacheReadUsdPer1M,
			cacheWriteUsdPer1M=excluded.cacheWriteUsdPer1M,
			currency=excluded.currency,
			source=excluded.source,
			updatedAt=excluded.updatedAt`,
		p.Provider, p.Model, p.InputUsdPer1M, p.OutputUsdPer1M, p.CacheReadUsdPer1M, p.CacheWriteUsdPer1M, p.Currency, p.Source, p.UpdatedAt.Format(time.RFC3339))
	return err
}

func DeletePricing(d *sql.DB, provider, model string) error {
	_, err := d.Exec(`DELETE FROM pricing WHERE provider = ? AND model = ?`, provider, model)
	return err
}

func SeedDefaultPricing(d *sql.DB) error {
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM pricing`).Scan(&n)
	if n > 0 {
		return nil
	}
	seed := []Pricing{

		{Provider: "anthropic", Model: "claude-fable-5", InputUsdPer1M: 10, OutputUsdPer1M: 50, CacheReadUsdPer1M: 1, CacheWriteUsdPer1M: 12.5, Source: "vendor-public-2026-06"},
		{Provider: "anthropic", Model: "claude-opus-4-8", InputUsdPer1M: 5, OutputUsdPer1M: 25, CacheReadUsdPer1M: 0.5, CacheWriteUsdPer1M: 6.25, Source: "vendor-public-2026-06"},
		{Provider: "anthropic", Model: "claude-sonnet-4-6", InputUsdPer1M: 3, OutputUsdPer1M: 15, CacheReadUsdPer1M: 0.3, CacheWriteUsdPer1M: 3.75, Source: "vendor-public-2026-06"},
		{Provider: "anthropic", Model: "claude-haiku-4-5", InputUsdPer1M: 1, OutputUsdPer1M: 5, CacheReadUsdPer1M: 0.1, CacheWriteUsdPer1M: 1.25, Source: "vendor-public-2026-06"},

		{Provider: "openai", Model: "gpt-5.5", InputUsdPer1M: 5, OutputUsdPer1M: 30, CacheReadUsdPer1M: 0.5, Source: "vendor-public-2026-06"},
		{Provider: "openai", Model: "gpt-5.4", InputUsdPer1M: 2.5, OutputUsdPer1M: 15, CacheReadUsdPer1M: 0.25, Source: "vendor-public-2026-06"},
		{Provider: "openai", Model: "gpt-5.4-mini", InputUsdPer1M: 0.75, OutputUsdPer1M: 4.5, CacheReadUsdPer1M: 0.075, Source: "vendor-public-2026-06"},
		{Provider: "openai", Model: "gpt-5.4-nano", InputUsdPer1M: 0.2, OutputUsdPer1M: 1.25, CacheReadUsdPer1M: 0.02, Source: "vendor-public-2026-06"},

		{Provider: "deepseek", Model: "deepseek-chat", InputUsdPer1M: 0.27, OutputUsdPer1M: 1.1, CacheReadUsdPer1M: 0.07, Source: "vendor-public-2026-06"},
		{Provider: "deepseek", Model: "deepseek-reasoner", InputUsdPer1M: 0.55, OutputUsdPer1M: 2.19, CacheReadUsdPer1M: 0.14, Source: "vendor-public-2026-06"},

		{Provider: "google", Model: "gemini-3.1-pro-preview", InputUsdPer1M: 2, OutputUsdPer1M: 12, Source: "vendor-public-2026-06"},
		{Provider: "google", Model: "gemini-3.5-flash", InputUsdPer1M: 1.5, OutputUsdPer1M: 9, Source: "vendor-public-2026-06"},
		{Provider: "google", Model: "gemini-3-flash-preview", InputUsdPer1M: 0.5, OutputUsdPer1M: 3, Source: "vendor-public-2026-06"},
		{Provider: "google", Model: "gemini-3.1-flash-lite", InputUsdPer1M: 0.25, OutputUsdPer1M: 1.5, Source: "vendor-public-2026-06"},
		{Provider: "groq", Model: "llama-3.3-70b-versatile", InputUsdPer1M: 0.59, OutputUsdPer1M: 0.79, Source: "vendor-public-2026-06"},
		{Provider: "kiro", Model: "kr/claude-sonnet-4.6", InputUsdPer1M: 0, OutputUsdPer1M: 0, Source: "free-tier"},
		{Provider: "kiro", Model: "kr/claude-opus-4.8", InputUsdPer1M: 0, OutputUsdPer1M: 0, Source: "free-tier"},
	}
	for i := range seed {
		if err := UpsertPricing(d, &seed[i]); err != nil {
			return err
		}
	}
	return nil
}
