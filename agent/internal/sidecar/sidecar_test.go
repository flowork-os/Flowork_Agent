package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// Validasi roadmap_sidecar agent: legacy-default = path lama persis; sidecar-active
// = laci content/data.

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"FLOWORK_SIDECAR", "FLOWORK_AGENTS_DIR", "FLOWORK_DATA_DIR"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestLegacyDefault(t *testing.T) {
	clearEnv(t)
	home, _ := os.UserHomeDir()
	if got, want := AgentsDir(), filepath.Join(home, ".flowork", "agents"); got != want {
		t.Errorf("AgentsDir legacy = %q, want %q", got, want)
	}
	if got, want := AppsDataDir(), filepath.Join(home, ".flowork", "apps"); got != want {
		t.Errorf("AppsDataDir legacy = %q, want %q", got, want)
	}
	if got, want := FloworkDB(), filepath.Join(home, ".flowork", "flowork.db"); got != want {
		t.Errorf("FloworkDB legacy = %q, want %q", got, want)
	}
	if Active() {
		t.Error("Active() harus false")
	}
}

func TestEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("FLOWORK_AGENTS_DIR", "/x/agents")
	if got, want := AgentsDir(), "/x/agents"; got != want {
		t.Errorf("AgentsDir($FLOWORK_AGENTS_DIR) = %q, want %q", got, want)
	}
	// AppsDataDir legacy turunan dari FLOWORK_AGENTS_DIR: Dir(/x/agents)/apps = /x/apps
	if got, want := AppsDataDir(), "/x/apps"; got != want {
		t.Errorf("AppsDataDir = %q, want %q", got, want)
	}
}

func TestSidecarActive(t *testing.T) {
	clearEnv(t)
	root := t.TempDir()
	t.Setenv("FLOWORK_SIDECAR", root)
	if !Active() {
		t.Fatal("Active() harus true")
	}
	if got, want := AgentsDir(), filepath.Join(root, "data", "agents"); got != want {
		t.Errorf("AgentsDir sidecar = %q, want %q", got, want)
	}
	if got, want := AppsDataDir(), filepath.Join(root, "data", "apps"); got != want {
		t.Errorf("AppsDataDir sidecar = %q, want %q", got, want)
	}
	if got, want := FloworkDB(), filepath.Join(root, "data", "flowork.db"); got != want {
		t.Errorf("FloworkDB sidecar = %q, want %q", got, want)
	}
	// AppsContentDir: butuh dir beneran di content/apps.
	ca := filepath.Join(root, "content", "apps")
	os.MkdirAll(ca, 0o755)
	if got, want := AppsContentDir(), ca; got != want {
		t.Errorf("AppsContentDir sidecar = %q, want %q", got, want)
	}
}
