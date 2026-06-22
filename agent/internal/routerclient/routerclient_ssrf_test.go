package routerclient

import "testing"

// Anti-SSRF: whitelist host HARUS nolak userinfo-bypass + host eksternal; loopback boleh.
func TestIsAllowedRouterURL_SSRF(t *testing.T) {
	allow := []string{
		"http://127.0.0.1:2402",
		"http://localhost",
		"https://127.0.0.1:8443",
		"http://0.0.0.0:2402",
	}
	deny := []string{
		"http://127.0.0.1:80@evil.com", // userinfo bypass (inti temuan audit)
		"http://127.0.0.1@evil.com",    // userinfo bypass
		"http://evil.com",
		"http://localhost.evil.com", // host beda, bukan localhost
		"http://127.0.0.1.evil.com",
		"http://[::1]:2402@evil.com",
	}
	for _, u := range allow {
		if !isAllowedRouterURL(u) {
			t.Errorf("HARUS allow loopback: %q", u)
		}
	}
	for _, u := range deny {
		if isAllowedRouterURL(u) {
			t.Errorf("HARUS tolak (SSRF): %q", u)
		}
	}
}

// New() fallback ke default kalau URL ga lolos whitelist (anti-exfil ke attacker host).
func TestNew_SSRFFallback(t *testing.T) {
	if got := New("http://127.0.0.1:80@evil.com").BaseURL; got != DefaultRouterURL {
		t.Fatalf("URL bypass harus fallback ke %q, dapat %q", DefaultRouterURL, got)
	}
	if got := New("http://127.0.0.1:2402").BaseURL; got != "http://127.0.0.1:2402" {
		t.Fatalf("URL loopback valid harus dipertahankan, dapat %q", got)
	}
}
