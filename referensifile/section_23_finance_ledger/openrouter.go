package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/teetah2402/flowork/internal/safeclient"
)

// OpenRouterKeyRequest represents the payload to create a new OpenRouter Key with limits.
type OpenRouterKeyRequest struct {
	Name  string  `json:"name"`
	Limit float64 `json:"limit,omitempty"` // USD limit, e.g. 0.50 for 50 cents
}

// OpenRouterKeyResponse represents the response containing the new key.
type OpenRouterKeyResponse struct {
	Key struct {
		Key   string   `json:"key"`
		Name  string   `json:"name"`
		Limit *float64 `json:"limit"`
		Usage float64  `json:"usage"`
	} `json:"key"`
}

// FinanceMinister mengatur keuangan API menggunakan OPENAI_API_MANAGEMENT_KEY.
type FinanceMinister struct {
	ManagementKey string
	BaseURL       string
	HTTPClient    *http.Client
	limiter       *RateLimiter
}

// ErrNoManagementKey is returned when OPENAI_API_MANAGEMENT_KEY is not set.
// Callers should check for this error and gracefully degrade (skip /balance
// features) instead of treating it as a fatal error.
var ErrNoManagementKey = fmt.Errorf("finance: OPENAI_API_MANAGEMENT_KEY not set — /balance features disabled")

// NewFinanceMinister mengembalikan instansi dari FinanceMinister.
// Kunci ini haruslah sebuah management key (bukan standard key).
// Returns ErrNoManagementKey if the env var is not set — callers should
// check for this and degrade gracefully (P4 Infra Debt fix).
func NewFinanceMinister() (*FinanceMinister, error) {
	key := os.Getenv("OPENAI_API_MANAGEMENT_KEY")
	if key == "" {
		return nil, ErrNoManagementKey
	}

	return &FinanceMinister{
		ManagementKey: key,
		BaseURL:       "https://openrouter.ai/api/v1",
		HTTPClient:    safeclient.NewClient(30 * time.Second),
		limiter:       NewRateLimiter(),
	}, nil
}

// MintSubKey akan membuat(minting) sekeping kunci API turunan dengan batasan biaya maksimal.
func (f *FinanceMinister) MintSubKey(ctx context.Context, agentName string, usdLimit float64) (string, error) {
	// SAFEGUARD per-transaksi: maks $6.00 per call
	if usdLimit > 6.0 {
		return "", fmt.Errorf("finance: PERMINTAAN DANA DITOLAK! Agen '%s' meminta $%.2f yang melanggar batas konstitusi $6.00 per-minta", agentName, usdLimit)
	}

	// I-A.1: cek daily cap + cooldown sebelum lanjut
	if err := f.limiter.Check(usdLimit); err != nil {
		return "", err
	}

	// Tanda tangani riwayat transaksi (Tamper-Evident log Audit)
	if err := LogAudit(agentName, usdLimit); err != nil {
		return "", fmt.Errorf("finance: gagal memverifikasi integritas buku kas: %w", err)
	}

	reqBody := OpenRouterKeyRequest{
		Name:  fmt.Sprintf("flowork-%s-%d", agentName, time.Now().Unix()),
		Limit: usdLimit,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("finance: gagal merangkai payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.BaseURL+"/keys", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("finance: gagal merangkai http request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+f.ManagementKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("finance: gagal menghubungi bank OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		// rc121 fortress fix: cap 1MB — error payload pendek.
		errorOutput, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			errorOutput = []byte("(failed to read error body)")
		}
		return "", fmt.Errorf("finance: permintaan kunci ditolak (status %d): %s", resp.StatusCode, string(errorOutput))
	}

	var data OpenRouterKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("finance: gagal membaca respon JSON bank: %w", err)
	}

	// I-A.1: catat pengeluaran ke daily tracker setelah sukses
	f.limiter.Record(usdLimit)
	return data.Key.Key, nil
}
