package router

import "testing"

// TestCloakProfileOverride buktiin profil cloaking bisa diatur via env (fwswitch) — bukan
// hardcode. Default = nilai lama; env isi = override (live, call-time).
func TestCloakProfileOverride(t *testing.T) {
	if claudeToolSuffix() != "_cc" || claudeVersion() != "2.1.92" {
		t.Fatal("default profil harus = nilai lama")
	}
	if len(ccDecoyToolNames()) != len(defaultCCDecoyToolNames) {
		t.Fatal("default decoy harus = daftar lama")
	}
	t.Setenv("FLOWORK_CLOAK_SUFFIX", "_zz")
	t.Setenv("FLOWORK_CLOAK_VERSION", "9.9.9")
	t.Setenv("FLOWORK_CLOAK_DECOYS", "Alpha, Beta ,Gamma")
	if claudeToolSuffix() != "_zz" || claudeVersion() != "9.9.9" {
		t.Fatal("override suffix/version gagal")
	}
	got := ccDecoyToolNames()
	if len(got) != 3 || got[0] != "Alpha" || got[2] != "Gamma" {
		t.Fatalf("override decoy gagal: %v", got)
	}
}
