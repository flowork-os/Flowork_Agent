package main

import (
	"testing"

	"github.com/flowork-os/flowork_Router/internal/executors"
	"github.com/flowork-os/flowork_Router/internal/store"
)

// Header hook harus SUNTIK header captured + Bearer terfresh di atas base default.
func TestAntigravityInjectHeaders(t *testing.T) {
	t.Setenv("FLOWORK_ANTIGRAVITY_CAPTURE", "1")
	// Simulasi capture: persist token + header asli.
	persistAntigravityCreds("Bearer ya29.TOKENFRESH123", map[string]string{
		"User-Agent":       "google-cloud-code-assist/1.99.0",
		"X-Client-Version": "9.9.9",
	})

	base := map[string]string{"User-Agent": "google-cloud-code-assist/1.16.0", "Accept": "application/json"}
	out := antigravityInjectHeaders(base, &store.ProviderConnection{ID: antigravityProviderID})
	if out == nil {
		t.Fatal("hook harus balik header (capture ON)")
	}
	if out["User-Agent"] != "google-cloud-code-assist/1.99.0" {
		t.Errorf("UA captured harus menang: %q", out["User-Agent"])
	}
	if out["X-Client-Version"] != "9.9.9" {
		t.Errorf("X-Client-Version captured harus kesuntik: %q", out["X-Client-Version"])
	}
	if out["Authorization"] != "Bearer ya29.TOKENFRESH123" {
		t.Errorf("Bearer terfresh harus kesuntik: %q", out["Authorization"])
	}
	if out["Accept"] != "application/json" {
		t.Errorf("header base non-captured harus tetep: %q", out["Accept"])
	}

	// Provider auto harus ke-provision + advertise gemini-3.
	d, _ := store.Open()
	p, _ := store.GetProvider(d, antigravityProviderID)
	if p == nil || !p.IsActive || p.Provider != "antigravity" {
		t.Fatalf("provider auto harus ada+aktif: %+v", p)
	}
	models, _ := p.Data[store.CfgModels].([]any)
	found := false
	for _, m := range models {
		if m == "gemini-3" {
			found = true
		}
	}
	if !found {
		t.Errorf("provider harus advertise gemini-3: %v", models)
	}
}

// Capture OFF → hook balik nil (executor pakai default frozen).
func TestAntigravityCaptureDisabled(t *testing.T) {
	t.Setenv("FLOWORK_ANTIGRAVITY_CAPTURE", "0")
	if out := antigravityInjectHeaders(map[string]string{}, nil); out != nil {
		t.Errorf("capture OFF → hook harus nil, dapet %v", out)
	}
	// Hook seam ke-set (bukan nil) di init.
	if executors.AntigravityHeaderHook == nil {
		t.Error("executors.AntigravityHeaderHook harus ke-wire di init")
	}
}
