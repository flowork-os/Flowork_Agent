package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPKCEChallenge(t *testing.T) {
	// RFC 7636 test vector.
	v := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	got := pkceChallenge(v)
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got != want {
		t.Errorf("PKCE S256 salah: got %q want %q", got, want)
	}
}

func TestRandB64Unique(t *testing.T) {
	a, b := randB64(16), randB64(16)
	if a == b || a == "" {
		t.Errorf("randB64 harus acak non-kosong: %q %q", a, b)
	}
}

func TestExtractOAuthConfig_FromBinary(t *testing.T) {
	// Bikin file 'binary' palsu yg ngandung client_id + secret (anti-hardcode:
	// dibaca dari file, bukan konstanta).
	dir := t.TempDir()
	fake := filepath.Join(dir, "language_server")
	content := "junk\x00" +
		"1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com" +
		"\x00moretjunk\x00GOCSPX-FAKEsecretFAKEsecretFAKE12345\x00end"
	if err := os.WriteFile(fake, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWORK_ANTIGRAVITY_BIN", fake)
	sec := secretRe.FindString(content)
	if !strings.HasPrefix(sec, "GOCSPX-") || len(sec) != 35 {
		t.Errorf("client_secret ga ke-extract bener: %q (len %d)", sec, len(sec))
	}
	_ = fake
}

// Co-lokasi: binary punya 2 client_id, yang BENER = yg deket secret GOCSPX.
func TestExtractOAuth_ColocatesWithSecret(t *testing.T) {
	// client_id SALAH duluan (jauh), client_id BENER nempel secret.
	data := []byte(
		"884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com" +
			strings.Repeat("x", 5000) +
			"1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com" +
			"\x00GOCSPX-FAKEsecretFAKEsecretFAKE12345\x00")
	cid, secrets := extractOAuthFromBytes(data)
	if !strings.HasPrefix(cid, "1071006060591-") {
		t.Errorf("harus pilih client_id yg ko-lokasi secret, dapet: %q", cid)
	}
	if len(secrets) != 1 {
		t.Errorf("harus 1 secret: %v", secrets)
	}
}
