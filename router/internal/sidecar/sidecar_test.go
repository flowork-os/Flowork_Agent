package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Validasi roadmap_sidecar: (1) legacy-default (FLOWORK_SIDECAR kosong) = path lama
// PERSIS; (2) sidecar-active = laci content/data. Env di-clear biar deterministik.

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"FLOWORK_SIDECAR", "FLOW_ROUTER_DATA", "FLOW_ROUTER_BRAIN_DB", "FLOWORK_BRAIN_VINDEX", "FLOWORK_BRAIN_GGUF", "FLOWORK_LLAMA_BIN"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestLegacyDefault(t *testing.T) {
	clearEnv(t)
	home, _ := os.UserHomeDir()
	// BrainDB: tanpa env + (biasanya) tanpa exe-dir/brain → ~/.flow_router/brain/...
	wantBrain := filepath.Join(home, ".flow_router", "brain", "flowork-brain.sqlite")
	if got := BrainDB(); got != wantBrain && !strings.HasSuffix(got, filepath.Join("brain", "flowork-brain.sqlite")) {
		t.Errorf("BrainDB legacy = %q, want %q", got, wantBrain)
	}
	// DynamicSkillsDir: ~/.flow_router/skills
	if got, want := DynamicSkillsDir(), filepath.Join(home, ".flow_router", "skills"); got != want {
		t.Errorf("DynamicSkillsDir legacy = %q, want %q", got, want)
	}
	if Active() {
		t.Error("Active() harus false pas FLOWORK_SIDECAR kosong")
	}
}

func TestSidecarActive(t *testing.T) {
	clearEnv(t)
	root := t.TempDir()
	t.Setenv("FLOWORK_SIDECAR", root)
	if !Active() {
		t.Fatal("Active() harus true")
	}
	if got, want := BrainDB(), filepath.Join(root, "data", "brain", "flowork-brain.sqlite"); got != want {
		t.Errorf("BrainDB sidecar = %q, want %q", got, want)
	}
	if got, want := DynamicSkillsDir(), filepath.Join(root, "data", "skills"); got != want {
		t.Errorf("DynamicSkillsDir sidecar = %q, want %q", got, want)
	}
	if got, want := Vindex(), filepath.Join(root, "data", "brain", "brain.vindex"); got != want {
		t.Errorf("Vindex sidecar = %q, want %q", got, want)
	}
	// ModelGGUF & LlamaBin: butuh file beneran (fileExists). Bikin di content/.
	cm := filepath.Join(root, "content", "models")
	os.MkdirAll(cm, 0o755)
	os.WriteFile(filepath.Join(cm, "flowork-brain.gguf"), []byte("x"), 0o644)
	if got, want := ModelGGUF(), filepath.Join(cm, "flowork-brain.gguf"); got != want {
		t.Errorf("ModelGGUF sidecar = %q, want %q", got, want)
	}
	cb := filepath.Join(root, "content", "bin")
	os.MkdirAll(cb, 0o755)
	os.WriteFile(filepath.Join(cb, "llama-server"), []byte("x"), 0o755)
	if got, want := LlamaBin(), filepath.Join(cb, "llama-server"); got != want {
		t.Errorf("LlamaBin sidecar = %q, want %q", got, want)
	}
}

func TestEnvOverrideStillWins(t *testing.T) {
	clearEnv(t)
	// Legacy env masih dihormatin pas sidecar OFF (zero regression).
	t.Setenv("FLOW_ROUTER_DATA", "/custom")
	if got, want := DynamicSkillsDir(), filepath.Join("/custom", "skills"); got != want {
		t.Errorf("DynamicSkillsDir($FLOW_ROUTER_DATA) = %q, want %q", got, want)
	}
	if got, want := BrainDB(), filepath.Join("/custom", "brain", "flowork-brain.sqlite"); got != want {
		t.Errorf("BrainDB($FLOW_ROUTER_DATA) = %q, want %q", got, want)
	}
}
