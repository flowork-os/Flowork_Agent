package floworkauth

import (
	"net/http"
	"testing"
)

// TestDriveByLocalhostDefense — loopback-gated sensitive endpoint (tools/run = exec)
// HARUS nolak request browser cross-site walau dari 127.0.0.1, tapi tetap loloskan
// GUI same-origin + caller non-browser (agent/curl). Ini pertahanan inti pre-freeze.
func TestDriveByLocalhostDefense(t *testing.T) {
	mk := func(secFetch, origin string) *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "/api/agents/tools/run?id=mr-flow", nil)
		r.RemoteAddr = "127.0.0.1:54321" // loopback peer
		r.Host = "127.0.0.1:1987"
		if secFetch != "" {
			r.Header.Set("Sec-Fetch-Site", secFetch)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	cases := []struct {
		name     string
		secFetch string
		origin   string
		wantPub  bool
	}{
		{"non-browser caller (agent/curl)", "", "", true},
		{"GUI same-origin", "same-origin", "http://127.0.0.1:1987", true},
		{"direct navigation (none)", "none", "", true},
		{"DRIVE-BY cross-site", "cross-site", "https://evil.com", false},
		{"DRIVE-BY same-site", "same-site", "", false},
		{"DRIVE-BY origin-mismatch (old browser)", "", "https://evil.com", false},
	}
	for _, c := range cases {
		if got := isPublicPath(mk(c.secFetch, c.origin)); got != c.wantPub {
			t.Errorf("%s: isPublicPath=%v, mau %v", c.name, got, c.wantPub)
		}
	}
}

// TestWebhookStaysCrossOrigin — webhook intake (secret-gated) HARUS tetap public
// walau cross-site (sistem eksternal/CCTV emang manggil lintas-origin).
func TestWebhookStaysCrossOrigin(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "/api/triggers/hook/abc", nil)
	r.RemoteAddr = "203.0.113.9:5000" // remote, bukan loopback
	r.Host = "flowork.example"
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("Origin", "https://some-service.com")
	if !isPublicPath(r) {
		t.Fatal("webhook intake harus tetap public (di-gate secret per-rule di handler)")
	}
}

// TestRemoteCannotReachLoopbackEndpoint — peer remote (non-loopback) ga boleh
// nembus endpoint loopback walau ga ada header browser.
func TestRemoteCannotReachLoopbackEndpoint(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "/api/agents/tools/run?id=mr-flow", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	r.Host = "flowork.example"
	if isPublicPath(r) {
		t.Fatal("endpoint loopback ga boleh public buat peer remote")
	}
}
