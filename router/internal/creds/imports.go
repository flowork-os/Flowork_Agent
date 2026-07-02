// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package creds

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var (
	errNoCreds     = errors.New("codex credentials not found — install Codex CLI + login")
	errNoToken     = errors.New("credential file present but no access token field recognised")
	errCursorVscdb = errors.New("cursor session is in state.vscdb (SQLite) — paste the token via the OAuth import tab")
)

type ImportStatus struct {
	Source    string `json:"source"`
	Path      string `json:"path"`
	Found     bool   `json:"found"`
	Expired   bool   `json:"expired"`
	MaskedKey string `json:"maskedKey,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

// extraDetectors — PAPAN COLOKAN (Pola A, Rule #7): detektor OAuth import baru
// (antigravity + CLI lain) dicolok dari file NON-frozen via RegisterDetector →
// file beku ini GA perlu dibuka lagi tiap nambah CLI. Plug-and-play: hapus
// sibling → CLI-nya ilang dari daftar, core utuh. Panic ext di-recover.
var extraDetectors []func(home string) ImportStatus

// RegisterDetector — colok 1 detektor kredensial CLI. Dipanggil init() sibling.
func RegisterDetector(f func(home string) ImportStatus) {
	if f != nil {
		extraDetectors = append(extraDetectors, f)
	}
}

func DetectAll() []ImportStatus {
	home, _ := os.UserHomeDir()
	out := []ImportStatus{
		detectClaude(home),
		detectCodex(home),
		detectCursor(home),
		detectGitlabDuo(home),
	}
	for _, f := range extraDetectors {
		func() {
			defer func() { _ = recover() }() // ext rusak ≠ daftar mati
			out = append(out, f(home))
		}()
	}
	// SEAM FILTER (non-frozen ngisi): rapihin daftar (mis. sembunyiin CLI untested
	// not-found). nil = apa adanya. Panic ext ke-recover.
	if DetectFilter != nil {
		func() {
			defer func() { _ = recover() }()
			if f := DetectFilter(out); f != nil {
				out = f
			}
		}()
	}
	return out
}

// DetectFilter — SEAM post-filter daftar import. Diisi file NON-frozen (imports_antigravity.go).
var DetectFilter func([]ImportStatus) []ImportStatus

func detectClaude(home string) ImportStatus {
	p := filepath.Join(home, ".claude", ".credentials.json")
	s := ImportStatus{Source: "claude-code", Path: p}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			s.Error = "not found — run `claude login`"
		} else {
			s.Error = err.Error()
		}
		return s
	}
	s.Found = true
	var c CredentialsFile
	if err := json.Unmarshal(data, &c); err != nil {
		s.Error = "parse: " + err.Error()
		return s
	}
	s.MaskedKey = maskToken(c.ClaudeAiOauth.AccessToken)
	if c.ClaudeAiOauth.ExpiresAt > 0 {
		exp := time.UnixMilli(c.ClaudeAiOauth.ExpiresAt)
		s.ExpiresAt = exp.Format(time.RFC3339)
		s.Expired = time.Now().After(exp)
	}
	return s
}

func detectCodex(home string) ImportStatus {
	candidates := []string{
		filepath.Join(home, ".codex", "auth.json"),
		filepath.Join(home, ".openai", "auth.json"),
	}
	s := ImportStatus{Source: "codex"}
	for _, p := range candidates {
		s.Path = p
		data, err := os.ReadFile(p)
		if err == nil {
			s.Found = true

			var auth map[string]any
			if json.Unmarshal(data, &auth) == nil {
				if tok, ok := auth["accessToken"].(string); ok {
					s.MaskedKey = maskToken(tok)
				} else if tok, ok := auth["token"].(string); ok {
					s.MaskedKey = maskToken(tok)
				}
				if exp, ok := auth["expiresAt"].(string); ok {
					s.ExpiresAt = exp
				}
			}
			return s
		}
	}
	s.Error = "not found — install Codex CLI + login"
	return s
}

func LoadCodexToken() (string, error) {
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".codex", "auth.json"),
		filepath.Join(home, ".openai", "auth.json"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var a map[string]any
		if json.Unmarshal(data, &a) != nil {
			continue
		}
		if t, ok := a["accessToken"].(string); ok && t != "" {
			return t, nil
		}
		if t, ok := a["token"].(string); ok && t != "" {
			return t, nil
		}
		if t, ok := a["OPENAI_API_KEY"].(string); ok && t != "" {
			return t, nil
		}
		if toks, ok := a["tokens"].(map[string]any); ok {
			if t, ok := toks["access_token"].(string); ok && t != "" {
				return t, nil
			}
		}
		return "", errNoToken
	}
	return "", errNoCreds
}

func LoadCursorToken() (string, error) {
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".cursor", "auth.json"))
	if err != nil {
		return "", errCursorVscdb
	}
	var a map[string]any
	if json.Unmarshal(data, &a) != nil {
		return "", errNoToken
	}
	for _, k := range []string{"accessToken", "token", "access_token"} {
		if t, ok := a[k].(string); ok && t != "" {
			return t, nil
		}
	}
	return "", errNoToken
}

func detectCursor(home string) ImportStatus {
	candidates := []string{
		filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb"),
		filepath.Join(home, ".cursor", "auth.json"),
	}
	s := ImportStatus{Source: "cursor"}
	for _, p := range candidates {
		s.Path = p
		if _, err := os.Stat(p); err == nil {
			s.Found = true

			s.MaskedKey = "(session present, parse Phase 2)"
			return s
		}
	}
	s.Error = "not found — install Cursor + login"
	return s
}

func detectGitlabDuo(home string) ImportStatus {
	p := filepath.Join(home, ".config", "gitlab-duo", "auth.json")
	s := ImportStatus{Source: "gitlab-duo", Path: p}
	if _, err := os.Stat(p); err != nil {
		s.Error = "not found"
		return s
	}
	s.Found = true
	s.MaskedKey = "(token present, parse Phase 2)"
	return s
}

func maskToken(t string) string {
	if len(t) < 14 {
		return "[masked]"
	}
	return t[:10] + "...[masked " + lenStr(len(t)) + " chars]"
}

func lenStr(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
