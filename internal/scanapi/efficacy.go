// efficacy.go — LAPIS EFIKASI: saringan FALSE-POSITIVE. Jalanin check privat lawan
// target BERSIH (dijamin ga vuln) → apapun yg NEMBAK = false-positive → karantina.
// Naikin "valid sintaks" → "ga asal nembak". 2 target bersih (HTML minimal +
// kaya-kata) biar nyaring matcher status-only DAN kata-umum.
//
// AMAN: target = server lokal sementara (httptest, 127.0.0.1) yg KITA bikin —
// bukan nyerang siapa-siapa. Owner-only loopback. nuclei tanpa -code (inert).
//
//	POST /api/scanner/efficacy  → {screened, fired, quarantined, remaining}

package scanapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// benignHandler — respons target BERSIH (nol vuln). rich=true → banyak kata umum
// (welcome/admin/login/config/version/…) buat nyaring matcher "kata-asal".
func benignHandler(rich bool) http.HandlerFunc {
	page := `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Test Page</h1></body></html>`
	if rich {
		page = `<!DOCTYPE html><html><head><title>Welcome</title><meta name="generator" content="WebServer"></head>` +
			`<body><h1>Welcome</h1><p>Home Dashboard Login Admin Settings Configuration Version Status Server ` +
			`Index Default API User Account Profile Management Console Panel Error Database Upload Download</p></body></html>`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
	}
}

func quarantineDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flowork", "scanner-quarantine")
}

func countYAML(dir string) int {
	n := 0
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			n++
		}
	}
	return n
}

// ScannerEfficacyHandler — POST: screen false-positive multi-target → karantina.
func ScannerEfficacyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		privDir := privateChecksDir()
		if privDir == "" {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dir check privat ga ketemu"})
			return
		}
		if _, err := exec.LookPath("nuclei"); err != nil {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "nuclei ga kepasang"})
			return
		}
		before := countYAML(privDir)
		if before == 0 {
			tfWriteJSON(w, 0, map[string]any{"ok": true, "screened": 0, "fired": 0, "quarantined": 0, "remaining": 0})
			return
		}

		// 2 target BERSIH sementara (lokal).
		s1 := httptest.NewServer(benignHandler(false))
		defer s1.Close()
		s2 := httptest.NewServer(benignHandler(true))
		defer s2.Close()

		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Minute)
		defer cancel()
		out, _ := exec.CommandContext(ctx, "nuclei", "-t", privDir,
			"-u", s1.URL, "-u", s2.URL,
			"-jsonl", "-nc", "-disable-update-check", "-silent", "-no-interactsh").Output()

		// template yg NEMBAK = false-positive (target bersih).
		fired := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var j struct {
				TemplatePath string `json:"template-path"`
			}
			if json.Unmarshal([]byte(line), &j) == nil && j.TemplatePath != "" {
				fired[j.TemplatePath] = true
			}
		}

		qdir := quarantineDir()
		moved := 0
		if qdir != "" {
			_ = os.MkdirAll(qdir, 0o755)
			for tp := range fired {
				if filepath.Dir(tp) == privDir {
					if os.Rename(tp, filepath.Join(qdir, filepath.Base(tp))) == nil {
						moved++
					}
				}
			}
		}
		if moved > 0 {
			resetNucleiPackCache()
		}
		tfWriteJSON(w, 0, map[string]any{
			"ok": true, "screened": before, "fired": len(fired),
			"quarantined": moved, "remaining": countYAML(privDir),
		})
	}
}
