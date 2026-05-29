package tools

// interceptors_kernel.go — KernelCapabilityInterceptor.
// Mencegat eksekusi tool untuk minta izin ke Kernel via /v1/tool/capability_check.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// KernelCapabilityInterceptor mencegat eksekusi tool untuk minta izin ke Kernel.
//
// ForwardExecution + Token are accepted for forward-compat with callsites
// that wire P-2 funnel mode and bearer auth (cmd/flowork-mcp). The Before()
// path itself does not yet branch on ForwardExecution; it simply performs
// a capability_check call. Callers may set the fields without affecting
// current behaviour.
type KernelCapabilityInterceptor struct {
	KernelURL        string
	WargaID          string
	ForwardExecution bool
	Token            string
}

// Before memeriksa izin ke Kernel.
func (i KernelCapabilityInterceptor) Before(ctx context.Context, invocation *Invocation) error {
	if i.KernelURL == "" {
		// Jika tidak ada URL kernel, fallback bypass (mis. dev env lokal tanpa kernel).
		return nil
	}

	reqBody, err := json.Marshal(map[string]string{
		"tool":     invocation.ToolName,
		"warga_id": i.WargaID,
	})
	if err != nil {
		return fmt.Errorf("gagal marshal capability request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.KernelURL+"/v1/tool/capability_check", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("gagal buat request capability check: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("gagal call kernel capability check: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("izin eksekusi tool %q ditolak oleh kernel: %s", invocation.ToolName, string(body))
	}

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("gagal parse response kernel capability check: %w", err)
	}

	if !result.Allowed {
		return fmt.Errorf("izin eksekusi tool %q ditolak oleh kernel (allowed=false)", invocation.ToolName)
	}

	return nil
}

func (KernelCapabilityInterceptor) After(_ context.Context, _ Invocation, _ *Result, _ error) {}
