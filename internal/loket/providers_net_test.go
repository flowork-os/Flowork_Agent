package loket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFetchSSRFGuard(t *testing.T) {
	// httptest always binds 127.0.0.1, so this doubles as the SSRF-guard test:
	// the request must be REFUSED, not served.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should not be reached"))
	}))
	defer srv.Close()
	_, err := httpFetch(context.Background(), "m", json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Errorf("loopback host must be blocked, got err=%v", err)
	}
}

func TestHTTPFetchValidation(t *testing.T) {
	if _, err := httpFetch(context.Background(), "m", json.RawMessage(`{}`)); err == nil {
		t.Error("empty url should error")
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, h := range []string{"localhost", "127.0.0.1", "127.1.2.3", "::1", "0.0.0.0", ""} {
		if !isLoopbackHost(h) {
			t.Errorf("%q should be loopback", h)
		}
	}
	for _, h := range []string{"example.com", "8.8.8.8", "api.telegram.org"} {
		if isLoopbackHost(h) {
			t.Errorf("%q should NOT be loopback", h)
		}
	}
}
