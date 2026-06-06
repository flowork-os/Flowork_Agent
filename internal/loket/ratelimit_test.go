package loket

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func okProvider(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func TestRateLimit(t *testing.T) {
	k := NewKernel()
	_ = k.Register("log", okProvider)
	k.SetRateLimit(3) // 3 calls per minute per module
	for i := 0; i < 3; i++ {
		if r := k.Call(context.Background(), "m", "log", nil); !r.OK {
			t.Fatalf("call %d should pass: %s", i, r.Error)
		}
	}
	if r := k.Call(context.Background(), "m", "log", nil); r.OK || !strings.Contains(r.Error, "rate limit") {
		t.Errorf("4th call should be rate-limited: ok=%v err=%q", r.OK, r.Error)
	}
	// Per-module: a different module is unaffected.
	if r := k.Call(context.Background(), "other", "log", nil); !r.OK {
		t.Errorf("other module should not be limited: %s", r.Error)
	}
}

func TestArgsSizeLimit(t *testing.T) {
	k := NewKernel()
	_ = k.Register("log", okProvider)
	big := json.RawMessage(`{"x":"` + strings.Repeat("a", maxArgsBytes+10) + `"}`)
	if r := k.Call(context.Background(), "m", "log", big); r.OK || !strings.Contains(r.Error, "too large") {
		t.Errorf("oversized args should be rejected: ok=%v err=%q", r.OK, r.Error)
	}
}

func TestRateLimitOffByDefault(t *testing.T) {
	k := NewKernel()
	_ = k.Register("log", okProvider)
	for i := 0; i < 2000; i++ {
		if r := k.Call(context.Background(), "m", "log", nil); !r.OK {
			t.Fatalf("default (no limit) call %d failed: %s", i, r.Error)
		}
	}
}
