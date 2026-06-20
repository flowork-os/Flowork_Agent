// Package sidecar — SUMBER KEBENARAN PATH tunggal untuk router (roadmap_sidecar.md,
// owner 2026-06-20). Kembaran agent/internal/sidecar (modul terpisah & sengaja
// independen → kode diduplikat, KONTRAK layout di-share):
//
//	$FLOWORK_SIDECAR/
//	├── content/   🟢 shippable (models GGUF, bin llama.cpp, brain-seed)
//	└── data/      🔴 user (brain-memori/WAL, skills authored, db)
//
// KOMPAT: env FLOWORK_SIDECAR kosong → semua resolver balikin path LEGACY persis
// (chain $FLOW_ROUTER_* / exe-dir / ~/.flow_router) → router live + build sekarang
// ZERO keganggu. Build baru (Fase 4) yg set FLOWORK_SIDECAR. Skills BAWAAN tetap
// embedded di binary (//go:embed) — cuma authored (data) yg lewat sini.
package sidecar

import (
	"os"
	"path/filepath"
	"strings"
)

// Root — akar sidecar kalau aktif, "" kalau legacy. FLOWORK_HOME SENGAJA belum
// di-alias (masih dipakai make-portable sbg data-home flat) — Fase 4 yg nyambungin.
func Root() string { return strings.TrimSpace(os.Getenv("FLOWORK_SIDECAR")) }

// Active — true kalau layout sidecar dipakai.
func Active() bool { return Root() != "" }

// ContentDir — <root>/content/<parts...> kalau aktif, else "".
func ContentDir(parts ...string) string {
	if r := Root(); r != "" {
		return filepath.Join(append([]string{r, "content"}, parts...)...)
	}
	return ""
}

// DataDir — <root>/data/<parts...> kalau aktif, else "".
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

// ── RESOLVER ROUTER (legacy-default = chain lama PERSIS; sidecar kalau aktif) ──

// BrainDB — sqlite otak (seed-corpus + memori runtime mutated). 🔴 data (di-seed dari
// content pas pertama jalan, Fase 3). Legacy chain = brain.go DBPath (tanpa pathOverride;
// override tetap dipegang brain.go).
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

// Vindex — index vektor brain (derivable/rebuildable). 🔴 data. Legacy = semantic.go.
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

// ModelGGUF — bobot model lokal (read-only, GEDE). 🟢 content (Fase 3: eksternal/opsional).
// Legacy = runtime.go ResolveFloworkBrain (return "" kalau ga ketemu).
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

// LlamaBin — binary llama-server (engine). 🟢 content/bin. Legacy = runtime.go ResolveLlamaBin
// (return "" → caller fallback PATH).
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

// DynamicSkillsDir — skill yg ditulis runtime (SKILL.md). 🔴 data. Legacy = skills.go.
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
