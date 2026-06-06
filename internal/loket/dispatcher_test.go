package loket

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestProviderPanicContained(t *testing.T) {
	k := NewKernel()
	_ = k.Register("log", func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		panic("boom")
	})
	// A provider panic must NOT crash the kernel — it becomes an error Result.
	r := k.Call(context.Background(), "m", "log", nil)
	if r.OK || !strings.Contains(r.Error, "panicked") {
		t.Errorf("panic not contained: ok=%v err=%q", r.OK, r.Error)
	}
	// And the kernel still serves other calls afterward (B survives A's failure).
	_ = k.Register("time.now", func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ts":"now"}`), nil
	})
	if r2 := k.Call(context.Background(), "m", "time.now", nil); !r2.OK {
		t.Error("kernel broken after a provider panic")
	}
}

// echoProvider returns the module id + args so tests can assert what the kernel
// passed through (proving the caller identity reaches the provider).
func echoProvider(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(fmt.Sprintf(`{"module":%q,"args":%s}`, module, args)), nil
}

func TestCallAutoCapAlwaysAllowed(t *testing.T) {
	k := NewKernel()
	_ = k.Register("log", echoProvider) // "log" is GrantAuto
	// No grant given on purpose — auto caps need none.
	r := k.Call(context.Background(), "any-mod", "log", json.RawMessage(`{"msg":"hi"}`))
	if !r.OK {
		t.Fatalf("auto cap refused: %s", r.Error)
	}
}

func TestCallOwnerCapNeedsGrant(t *testing.T) {
	k := NewKernel()
	_ = k.Register("exec.run", echoProvider) // GrantOwner

	// Without a grant → refused.
	if r := k.Call(context.Background(), "scanner-x", "exec.run", nil); r.OK {
		t.Error("exec.run allowed without grant")
	}
	// After grant → allowed.
	k.Grant("scanner-x", []string{"exec.run"})
	if r := k.Call(context.Background(), "scanner-x", "exec.run", nil); !r.OK {
		t.Errorf("exec.run refused after grant: %s", r.Error)
	}
}

func TestCallUnknownCapRefused(t *testing.T) {
	k := NewKernel()
	if r := k.Call(context.Background(), "m", "fly.to.moon", nil); r.OK {
		t.Error("unknown cap allowed")
	}
}

func TestCallModuleIdentityReachesProvider(t *testing.T) {
	k := NewKernel()
	_ = k.Register("store.kv.get", echoProvider)
	r := k.Call(context.Background(), "title-writer", "store.kv.get", json.RawMessage(`{"k":"x"}`))
	if !r.OK {
		t.Fatalf("call failed: %s", r.Error)
	}
	var got struct {
		Module string `json:"module"`
	}
	_ = json.Unmarshal(r.Result, &got)
	if got.Module != "title-writer" {
		t.Errorf("provider saw module %q, want title-writer", got.Module)
	}
}

func TestServiceSwap(t *testing.T) {
	k := NewKernel()
	_ = k.Register("llm.complete", func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"engine":"cloud"}`), nil
	})
	// Swap to a "local model" provider — same cap, new provider, no kernel change.
	_ = k.Register("llm.complete", func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"engine":"local"}`), nil
	})
	r := k.Call(context.Background(), "m", "llm.complete", nil)
	if !r.OK || string(r.Result) != `{"engine":"local"}` {
		t.Errorf("service swap failed: ok=%v result=%s", r.OK, r.Result)
	}
}

func TestProviderErrorBecomesNotOK(t *testing.T) {
	k := NewKernel()
	_ = k.Register("http.fetch", func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("network down")
	})
	k.Grant("m", []string{"http.fetch"})
	r := k.Call(context.Background(), "m", "http.fetch", nil)
	if r.OK || r.Error != "network down" {
		t.Errorf("provider error not surfaced: ok=%v err=%q", r.OK, r.Error)
	}
}

func TestMissingProviderForKnownCap(t *testing.T) {
	k := NewKernel()
	// "time.now" is in the Catalog (auto) but no provider registered.
	r := k.Call(context.Background(), "m", "time.now", nil)
	if r.OK {
		t.Error("call succeeded with no provider")
	}
}

func TestGrantManifestAndRevoke(t *testing.T) {
	k := NewKernel()
	_ = k.Register("fs.write", echoProvider) // GrantOwner
	m := &Manifest{ID: "writer-mod", Kind: KindAgent, Name: "W", Entry: "handle",
		Consumes: []string{"fs.write"}}
	k.GrantManifest(m)
	if r := k.Call(context.Background(), "writer-mod", "fs.write", nil); !r.OK {
		t.Fatalf("granted manifest cap refused: %s", r.Error)
	}
	k.Revoke("writer-mod")
	if r := k.Call(context.Background(), "writer-mod", "fs.write", nil); r.OK {
		t.Error("cap still allowed after revoke")
	}
}

// TestServiceProvidedCapOutsideCatalog — a service may provide a NEW cap name
// (extending the namespace); a consumer that was granted it can call it.
func TestServiceProvidedCapOutsideCatalog(t *testing.T) {
	k := NewKernel()
	_ = k.Register("desktop.screenshot", echoProvider) // not in frozen Catalog
	// Not granted → refused.
	if r := k.Call(context.Background(), "op", "desktop.screenshot", nil); r.OK {
		t.Error("ungranted service cap allowed")
	}
	// Granted → allowed.
	k.Grant("op", []string{"desktop.screenshot"})
	if r := k.Call(context.Background(), "op", "desktop.screenshot", nil); !r.OK {
		t.Errorf("granted service cap refused: %s", r.Error)
	}
}
