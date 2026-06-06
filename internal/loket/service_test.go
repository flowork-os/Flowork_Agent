package loket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServiceManifestDrivenGrant — a module that declares it consumes an owner
// capability in its loket.json gets that capability granted the first time it
// calls, with zero kernel code per module.
func TestServiceManifestDrivenGrant(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "scanner-x")
	_ = os.MkdirAll(modDir, 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "loket.json"),
		[]byte(`{"id":"scanner-x","kind":"scanner","name":"X","entry":"handle","consumes":["fs.read"]}`), 0o644)

	s := NewService(Deps{
		StorePath: func(m string) (string, error) { return filepath.Join(dir, m+".db"), nil },
		ModuleDir: func(m string) (string, error) { return filepath.Join(dir, m), nil },
	}, "")
	_ = s.Kernel.Register("fs.read", okProvider)

	// Before any service call, the grant has not been applied → refused.
	if r := s.Kernel.Call(context.Background(), "scanner-x", "fs.read", nil); r.OK {
		t.Error("fs.read should be refused before the manifest grant is applied")
	}
	// Calling through the Service applies the manifest grant → now allowed.
	if r := doCall(t, s, "scanner-x", "", "fs.read", `{}`); !r.OK {
		t.Errorf("fs.read refused after manifest grant: %s", r.Error)
	}
}

// TestServiceManifestTierRuleHeld — an extension whose loket.json asks for a
// primary-only cap is NOT granted it (ParseManifest rejects the manifest), so the
// S1 tier boundary survives even via the grant path.
func TestServiceManifestTierRuleHeld(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "crypto-x")
	_ = os.MkdirAll(modDir, 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "loket.json"),
		[]byte(`{"id":"crypto-x","kind":"agent","name":"X","entry":"handle","tier":"extension","consumes":["brain.shared.search"]}`), 0o644)

	s := NewService(Deps{
		StorePath: func(m string) (string, error) { return filepath.Join(dir, m+".db"), nil },
		ModuleDir: func(m string) (string, error) { return filepath.Join(dir, m), nil },
	}, "")
	_ = s.Kernel.Register("brain.shared.search", okProvider)

	if r := doCall(t, s, "crypto-x", "", "brain.shared.search", `{}`); r.OK {
		t.Error("extension must NOT be granted the primary-only shared brain via a malformed manifest")
	}
}

// TestServiceSelfDeclaredPrimaryRefused — a module that writes tier:"primary" in
// its OWN loket.json but is NOT authoritatively primary (IsPrimary=false) must NOT
// be granted the tier-gated shared corpus. The manifest's tier is a claim; the
// kernel's allowlist is the authority. Closes the self-promotion hole.
func TestServiceSelfDeclaredPrimaryRefused(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "sneaky")
	_ = os.MkdirAll(modDir, 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "loket.json"),
		[]byte(`{"id":"sneaky","kind":"agent","name":"X","entry":"handle","tier":"primary","consumes":["brain.shared.search"]}`), 0o644)

	s := NewService(Deps{
		StorePath: func(m string) (string, error) { return filepath.Join(dir, m+".db"), nil },
		ModuleDir: func(m string) (string, error) { return filepath.Join(dir, m), nil },
		IsPrimary: func(string) bool { return false }, // authoritative: NOT primary
	}, "")
	_ = s.Kernel.Register("brain.shared.search", okProvider)

	if r := doCall(t, s, "sneaky", "", "brain.shared.search", `{}`); r.OK {
		t.Error("self-declared primary must NOT reach the shared corpus when IsPrimary=false")
	}
}

// TestServiceAuthoritativePrimaryGranted — the SAME manifest IS granted the shared
// corpus when the kernel's authoritative allowlist confirms the module is primary.
func TestServiceAuthoritativePrimaryGranted(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "mr-flow-next")
	_ = os.MkdirAll(modDir, 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "loket.json"),
		[]byte(`{"id":"mr-flow-next","kind":"agent","name":"X","entry":"handle","tier":"primary","consumes":["brain.shared.search"]}`), 0o644)

	s := NewService(Deps{
		StorePath: func(m string) (string, error) { return filepath.Join(dir, m+".db"), nil },
		ModuleDir: func(m string) (string, error) { return filepath.Join(dir, m), nil },
		IsPrimary: func(m string) bool { return m == "mr-flow-next" },
	}, "")
	_ = s.Kernel.Register("brain.shared.search", okProvider)

	if r := doCall(t, s, "mr-flow-next", "", "brain.shared.search", `{}`); !r.OK {
		t.Errorf("authoritative primary should reach the shared corpus: %s", r.Error)
	}
}

func testService(t *testing.T, secret string) *Service {
	t.Helper()
	dir := t.TempDir()
	return NewService(Deps{
		StorePath: func(module string) (string, error) {
			return filepath.Join(dir, module+".db"), nil
		},
		Send:   func(context.Context, string, Message) error { return nil },
		Invoke: func(context.Context, string, Message) (json.RawMessage, error) { return json.RawMessage(`{}`), nil },
	}, secret)
}

func doCall(t *testing.T, s *Service, module, secret, capName, args string) Result {
	t.Helper()
	body := `{"cap":"` + capName + `","args":` + args + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/kernel/call", strings.NewReader(body))
	if secret != "" {
		req.Header.Set("X-Flowork-Secret", secret)
		req.Header.Set("X-Flowork-Caller", module)
	} else {
		req.URL.RawQuery = "module=" + module
	}
	rec := httptest.NewRecorder()
	s.CallHandler(rec, req)
	var res Result
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	return res
}

func TestServiceDevModeRoundTrip(t *testing.T) {
	s := testService(t, "") // dev: ?module= accepted
	if r := doCall(t, s, "writer", "", "store.kv.set", `{"k":"tone","v":"warm"}`); !r.OK {
		t.Fatalf("set failed: %s", r.Error)
	}
	r := doCall(t, s, "writer", "", "store.kv.get", `{"k":"tone"}`)
	if !r.OK {
		t.Fatalf("get failed: %s", r.Error)
	}
	var got struct {
		Value string `json:"value"`
	}
	_ = json.Unmarshal(r.Result, &got)
	if got.Value != "warm" {
		t.Errorf("round trip via HTTP wrong: %q", got.Value)
	}
}

func TestServiceVerifiedCallerPath(t *testing.T) {
	const secret = "s3cr3t"
	s := testService(t, secret)
	// With the right secret + caller header → identified, call works.
	if r := doCall(t, s, "writer", secret, "store.kv.set", `{"k":"x","v":"1"}`); !r.OK {
		t.Fatalf("verified caller call failed: %s", r.Error)
	}
}

func TestServiceRejectsUnidentifiedWhenSecretSet(t *testing.T) {
	const secret = "s3cr3t"
	s := testService(t, secret)
	// Secret configured but request carries no secret/header → ?module= ignored.
	r := doCall(t, s, "spoofer", "", "store.kv.get", `{"k":"x"}`)
	if r.OK || !strings.Contains(r.Error, "unidentified") {
		t.Errorf("expected unidentified rejection, got ok=%v err=%q", r.OK, r.Error)
	}
}

func TestServiceMethodGuard(t *testing.T) {
	s := testService(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/kernel/call", nil)
	rec := httptest.NewRecorder()
	s.CallHandler(rec, req)
	var res Result
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.OK {
		t.Error("GET should be rejected")
	}
}
