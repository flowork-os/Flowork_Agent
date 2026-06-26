// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&iflowExecutor{}) }

type iflowExecutor struct{}

func (i *iflowExecutor) Name() string { return "iflow" }

func (i *iflowExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://apis.iflow.cn/v1"
	}
	return trimRightSlash(base) + "/chat/completions"
}

func iflowSignature(userAgent, sessionID string, timestampMs int64, apiKey string) string {
	msg := userAgent + sessionID + strconv.FormatInt(timestampMs, 10)
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

func (i *iflowExecutor) headers(p *store.ProviderConnection) map[string]string {
	sessionID := uuid.NewString()
	ua := "iFlow-CLI/1.0"
	ts := time.Now().UnixMilli()
	apiKey, _ := p.Data[store.CfgAPIKey].(string)
	sig := iflowSignature(ua, sessionID, ts, apiKey)
	h := map[string]string{
		"User-Agent":    ua,
		"X-Session-ID":  sessionID,
		"X-Timestamp":   strconv.FormatInt(ts, 10),
		"X-Signature":   sig,
		"Authorization": "Bearer " + apiKey,
	}
	return h
}

func (i *iflowExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, i.endpoint(p), MarshalRequest(req), i.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (i *iflowExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, i.endpoint(p), MarshalRequest(req), i.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}
