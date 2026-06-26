// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package sidecar

import (
	"os"
	"path/filepath"
	"strings"
)

func Root() string { return strings.TrimSpace(os.Getenv("FLOWORK_SIDECAR")) }

func Active() bool { return Root() != "" }

func ContentDir(parts ...string) string {
	if r := Root(); r != "" {
		return filepath.Join(append([]string{r, "content"}, parts...)...)
	}
	return ""
}

func DataDir(parts ...string) string {
	if r := Root(); r != "" {
		return filepath.Join(append([]string{r, "data"}, parts...)...)
	}
	return ""
}

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/tmp"
}

func exeDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return ""
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func BrainDB() string {
	if d := DataDir("brain"); d != "" {
		return filepath.Join(d, "flowork-brain.sqlite")
	}
	if p := strings.TrimSpace(os.Getenv("FLOW_ROUTER_BRAIN_DB")); p != "" {
		return p
	}
	if d := strings.TrimSpace(os.Getenv("FLOW_ROUTER_DATA")); d != "" {
		return filepath.Join(d, "brain", "flowork-brain.sqlite")
	}
	if ed := exeDir(); ed != "" {
		if p := filepath.Join(ed, "brain", "flowork-brain.sqlite"); fileExists(p) {
			return p
		}
	}
	return filepath.Join(home(), ".flow_router", "brain", "flowork-brain.sqlite")
}

func Vindex() string {
	if d := DataDir("brain"); d != "" {
		return filepath.Join(d, "brain.vindex")
	}
	if p := strings.TrimSpace(os.Getenv("FLOWORK_BRAIN_VINDEX")); p != "" {
		return p
	}
	if ed := exeDir(); ed != "" {
		if p := filepath.Join(ed, "brain", "brain.vindex"); fileExists(p) {
			return p
		}
	}
	return filepath.Join("brain", "brain.vindex")
}

func ModelGGUF() string {
	if d := ContentDir("models"); d != "" {
		if p := filepath.Join(d, "flowork-brain.gguf"); fileExists(p) {
			return p
		}
	}
	if p := strings.TrimSpace(os.Getenv("FLOWORK_BRAIN_GGUF")); p != "" && fileExists(p) {
		return p
	}
	var cands []string
	if ed := exeDir(); ed != "" {
		cands = append(cands,
			filepath.Join(ed, "models", "flowork-brain.gguf"),
			filepath.Join(ed, "..", "router", "models", "flowork-brain.gguf"),
		)
	}
	cands = append(cands, filepath.Join("router", "models", "flowork-brain.gguf"))
	for _, c := range cands {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func LlamaBin() string {
	if d := ContentDir("bin"); d != "" {
		for _, c := range []string{filepath.Join(d, "llama-server"), filepath.Join(d, "llama-server.exe")} {
			if fileExists(c) {
				return c
			}
		}
	}
	if p := strings.TrimSpace(os.Getenv("FLOWORK_LLAMA_BIN")); p != "" && fileExists(p) {
		return p
	}
	if ed := exeDir(); ed != "" {
		for _, c := range []string{
			filepath.Join(ed, "bin", "llama-server"), filepath.Join(ed, "bin", "llama-server.exe"),
			filepath.Join(ed, "llama-server"), filepath.Join(ed, "llama-server.exe"),
		} {
			if fileExists(c) {
				return c
			}
		}
	}
	return ""
}

func DynamicSkillsDir() string {
	if d := DataDir("skills"); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("FLOW_ROUTER_DATA")); d != "" {
		return filepath.Join(d, "skills")
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".flow_router", "skills")
}
