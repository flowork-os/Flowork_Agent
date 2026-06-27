package apps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"flowork-gui/internal/apps/cliadapter"
)

// fakeAdapter — bikin file adapter palsu + arahin resolver ke situ (test ga butuh binary asli).
func fakeAdapter(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fw-app-adapter")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := resolveAdapterBin
	resolveAdapterBin = func() (string, error) { return bin, nil }
	t.Cleanup(func() { resolveAdapterBin = old })
}

func TestSlugID(t *testing.T) {
	cases := map[string]string{
		"https://github.com/yt-dlp/yt-dlp.git": "yt-dlp",
		"/home/x/My Cool Repo":                 "my-cool-repo",
		"yt-dlp":                               "yt-dlp",
	}
	for in, want := range cases {
		if got := SlugID(in); got != want {
			t.Errorf("SlugID(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestAdoptLocalFolder(t *testing.T) {
	fakeAdapter(t)
	// repo source lokal: python tanpa dep (skip install).
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "main.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	appsDir := t.TempDir()
	m := NewManager(appsDir)

	res, err := m.AdoptRepo(context.Background(), srcDir, "mytool", true, true /*skipInstall*/, false)
	if err != nil {
		t.Fatalf("AdoptRepo: %v", err)
	}
	if !res.Live || res.ID != "mytool" || res.Runtime != "python" {
		t.Fatalf("hasil ga sesuai: %+v", res)
	}

	// app harus LIVE di manager.
	if _, ok := m.Get("mytool"); !ok {
		t.Fatal("app 'mytool' ga ke-register di manager")
	}

	// manifest.json + adapter.json + repo/ kebentuk.
	base := filepath.Join(appsDir, "mytool")
	for _, f := range []string{"manifest.json", "adapter.json", filepath.Join("repo", "main.py")} {
		if _, err := os.Stat(filepath.Join(base, f)); err != nil {
			t.Errorf("file %s ga ada: %v", f, err)
		}
	}

	// adapter.json valid + punya op "run" args_list yg nunjuk venv python.
	raw, _ := os.ReadFile(filepath.Join(base, "adapter.json"))
	var cfg cliadapter.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("adapter.json invalid: %v", err)
	}
	run, ok := cfg.Ops["run"]
	if !ok || run.ArgStyle != "args_list" {
		t.Fatalf("op 'run' args_list ga bener: %+v", cfg.Ops)
	}
	if cfg.Workdir != "repo" {
		t.Fatalf("workdir = %q, mau 'repo'", cfg.Workdir)
	}

	// manifest: op run tool:true (auto jadi tool agent).
	mraw, _ := os.ReadFile(filepath.Join(base, "manifest.json"))
	var man Manifest
	_ = json.Unmarshal(mraw, &man)
	if man.Kind != "app" || len(man.Operations) != 1 || !man.Operations[0].Tool {
		t.Fatalf("manifest ga sesuai: %+v", man)
	}
}

func TestAdoptRequiresConsent(t *testing.T) {
	fakeAdapter(t)
	m := NewManager(t.TempDir())
	_, err := m.AdoptRepo(context.Background(), t.TempDir(), "x", false /*approveExec*/, true, false)
	if err == nil {
		t.Fatal("mau error consent (approve_exec=false), dapet nil")
	}
}

func TestAdoptRejectExisting(t *testing.T) {
	fakeAdapter(t)
	src := t.TempDir()
	_ = os.WriteFile(filepath.Join(src, "go.mod"), []byte("module x\n"), 0o644)
	appsDir := t.TempDir()
	m := NewManager(appsDir)
	if _, err := m.AdoptRepo(context.Background(), src, "dup", true, true, false); err != nil {
		t.Fatalf("adopt pertama: %v", err)
	}
	if _, err := m.AdoptRepo(context.Background(), src, "dup", true, true, false /*force*/); err == nil {
		t.Fatal("mau error 'udah ada' tanpa force, dapet nil")
	}
	if _, err := m.AdoptRepo(context.Background(), src, "dup", true, true, true /*force*/); err != nil {
		t.Fatalf("adopt force mestinya sukses: %v", err)
	}
}
