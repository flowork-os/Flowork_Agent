// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (autonomy grant 2026-06-19).
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-19
// Reason: CGM router embedding client (/v1/embeddings) — built + unit-tested (build/vet/test green). Extend = new file, jangan modify ini.
//
// embed.go — agent → router embedding client (CGM entity-resolution, roadmap §4.4).
//
// Reuse existing Client (routerclient.go, locked) — file baru, JANGAN modify yg locked.
// Manggil router POST /v1/embeddings (OpenAI-compatible) buat dapet vektor "makna"
// dari label node → dipakai entity-resolution biar gak ada node kembar.
//
// Mesin embedding ada di ROUTER (bge-m3 dim 1024, vecindex) — agent pinjem hitungannya,
// hasilnya disimpen LOKAL (quantized) di agent state.db. Jadi graph + maknanya tetap
// portable ikut folder agent (D2), tapi compute embedding sentral di router.
//
// Degrade gracefully: kalau router/embedding model gak ada → return error; caller
// (cognitive_resolve.go) fallback ke resolusi non-embedding (label-exact).

package routerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultEmbedModel — model embedding default (bge-m3 = yg dipake vecindex router).
// Override lewat arg kalau Settings nunjuk model lain.
const DefaultEmbedModel = "bge-m3"

type embedReq struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// EmbedText minta 1 vektor embedding buat `text` dari router. model kosong → DefaultEmbedModel.
// Return []float32 (hemat; nanti di-quantize 8-bit buat storage).
func (c *Client) EmbedText(ctx context.Context, model, text string) ([]float32, error) {
	if c == nil {
		return nil, fmt.Errorf("router client nil")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("text kosong")
	}
	if model == "" {
		model = DefaultEmbedModel
	}

	body, err := json.Marshal(embedReq{Model: model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB cap (1024 floats fits)
	var out embedResp
	if uerr := json.Unmarshal(raw, &out); uerr != nil {
		return nil, fmt.Errorf("decode (status %d): %w", resp.StatusCode, uerr)
	}
	if resp.StatusCode >= 400 {
		if out.Error != nil {
			return nil, fmt.Errorf("router %d: %s", resp.StatusCode, out.Error.Message)
		}
		return nil, fmt.Errorf("router status %d", resp.StatusCode)
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding (model %q — configured di Settings?)", model)
	}

	vec := make([]float32, len(out.Data[0].Embedding))
	for i, f := range out.Data[0].Embedding {
		vec[i] = float32(f)
	}
	return vec, nil
}
