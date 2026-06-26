// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package pricing

import (
	"database/sql"
	"time"
)

func Calc(db *sql.DB, provider, model, tier string, inputToks, outputToks int) (float64, error) {
	if tier == "" {
		tier = "default"
	}
	var inputRate, outputRate float64
	err := db.QueryRow(
		`SELECT input_per_1m_usd, output_per_1m_usd
		 FROM pricing_rules
		 WHERE provider = ? AND model = ? AND tier = ? AND enabled = 1`,
		provider, model, tier).Scan(&inputRate, &outputRate)
	if err == sql.ErrNoRows {

		err = db.QueryRow(
			`SELECT input_per_1m_usd, output_per_1m_usd
			 FROM pricing_rules
			 WHERE provider = ? AND model = ? AND enabled = 1
			 ORDER BY (CASE WHEN tier = 'default' THEN 0 ELSE 1 END), id
			 LIMIT 1`,
			provider, model).Scan(&inputRate, &outputRate)
		if err == sql.ErrNoRows {
			return 0, nil
		}
	}
	if err != nil {
		return 0, err
	}
	inputUSD := float64(inputToks) / 1_000_000.0 * inputRate
	outputUSD := float64(outputToks) / 1_000_000.0 * outputRate
	return inputUSD + outputUSD, nil
}

func LogCall(db *sql.DB, caller, provider, model string, inputToks, outputToks int,
	costUSD float64, latencyMS int64, status string) error {
	if status == "" {
		status = "success"
	}
	_, err := db.Exec(
		`INSERT INTO provider_call_log
		   (caller, provider, model, input_tokens, output_tokens,
		    cost_usd, latency_ms, status, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		caller, provider, model, inputToks, outputToks,
		costUSD, latencyMS, status,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
