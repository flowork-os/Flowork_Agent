// scanner_checks.go — INGEST check privat (rumah distilasi 5jt + check komunitas).
//
// Check = 1 file .yaml gaya nuclei ("1 scaner 1 file"). Masuk lewat GERBANG
// `nuclei -validate` (sintaks) → disimpen di <nuclei-templates>/flowork-private/
// → OTOMATIS nyatu ke arsenal (keitung, install/uninstall, exclude) + ke-scan.
//
// AMAN (anti jadi senjata / ngerusak):
//   1. owner-only loopback (agent GA punya akses).
//   2. nuclei DIJALANIN TANPA `-code` → template protokol `code` (eksekusi kode)
//      INERT pas scan; cuma probe http/dns/tcp deklaratif yang jalan.
//   3. validate dulu lewat file TEMP — invalid = DITOLAK, ga pernah masuk arsenal.
//   4. nama check di-sanitize (anti path-traversal).
//
//	POST /api/scanner/checks/add    {name, yaml}
//	POST /api/scanner/checks/delete {name}

package scanapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// privateChecksDir — rumah check privat: subdir flowork-private di bawah dir
// template nuclei → auto-integrate ke arsenal + scan. "" kalau nuclei ga ketemu.
func privateChecksDir() string {
	base := nucleiTemplatesDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "flowork-private")
}

var checkNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func validCheckName(name string) bool {
	return name != "" && checkNameRe.MatchString(name) && !strings.Contains(name, "..")
}

// validateNucleiTemplate — GERBANG sintaks: `nuclei -validate -t <path>`. READ-ONLY
// (validate ga nge-scan / ga eksekusi template). err != nil → template invalid.
//
// CATATAN: exit code `-validate` SELALU 0 (ga reliable) → kita PARSE output.
// valid = "All templates validated successfully"; rusak = "Error occurred
// loading" / "Could not validate". `-nc` = no ANSI (parsing bersih).
func validateNucleiTemplate(path string) error {
	if _, err := exec.LookPath("nuclei"); err != nil {
		return nil // nuclei ga ada → skip gate graceful (check tetep ke-simpen)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "nuclei", "-validate", "-t", path, "-nc", "-disable-update-check").CombinedOutput()
	outStr := string(out)
	bad := strings.Contains(outStr, "Error occurred loading") ||
		strings.Contains(outStr, "Could not validate") ||
		strings.Contains(outStr, "errors occurred during template validation")
	if strings.Contains(outStr, "validated successfully") && !bad {
		return nil
	}
	msg := "template invalid (no valid request/matcher?)"
	for _, ln := range strings.Split(outStr, "\n") {
		if strings.Contains(ln, "Error occurred loading") || strings.Contains(ln, "cause=") {
			msg = strings.TrimSpace(ln)
			break
		}
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("nuclei -validate: %s", msg)
}

// ingestValidatedCheck — validate `yaml` lewat temp .yaml → kalau lolos, simpan ke
// <dir>/<name>.yaml. err != nil → DITOLAK (ga nyentuh dir privat). Dipakai bareng
// ingest manual (scanner_checks) + generator distilasi (distill.go).
func ingestValidatedCheck(dir, name, yaml string) error {
	tmpf, err := os.CreateTemp("", "flowork-check-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := tmpf.Name()
	defer os.Remove(tmpPath)
	if _, werr := tmpf.WriteString(yaml); werr != nil {
		tmpf.Close()
		return werr
	}
	tmpf.Close()
	if verr := validateNucleiTemplate(tmpPath); verr != nil {
		return verr
	}
	return os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(yaml), 0o644)
}

// ScannerCheckAddHandler — POST {name, yaml}: validate → simpan ke flowork-private.
func ScannerCheckAddHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Name string `json:"name"`
			YAML string `json:"yaml"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		name := strings.TrimSuffix(strings.TrimSpace(body.Name), ".yaml")
		if !validCheckName(name) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "name wajib [a-zA-Z0-9._-], no .."})
			return
		}
		if strings.TrimSpace(body.YAML) == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "yaml kosong"})
			return
		}
		dir := privateChecksDir()
		if dir == "" {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dir template nuclei ga ketemu (install nuclei-templates dulu)"})
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if verr := ingestValidatedCheck(dir, name, body.YAML); verr != nil {
			code := http.StatusUnprocessableEntity
			if !strings.HasPrefix(verr.Error(), "nuclei -validate") {
				code = http.StatusInternalServerError
			}
			tfWriteJSON(w, code, map[string]any{"error": verr.Error()})
			return
		}
		resetNucleiPackCache()
		tfWriteJSON(w, 0, map[string]any{"ok": true, "name": name, "path": "flowork-private/" + name + ".yaml"})
	}
}

// ScannerCheckDeleteHandler — POST {name}: hapus check privat dari arsenal.
func ScannerCheckDeleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		name := strings.TrimSuffix(strings.TrimSpace(body.Name), ".yaml")
		if !validCheckName(name) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "name invalid"})
			return
		}
		dir := privateChecksDir()
		if dir == "" {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dir ga ketemu"})
			return
		}
		if err := os.Remove(filepath.Join(dir, name+".yaml")); err != nil {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		resetNucleiPackCache()
		tfWriteJSON(w, 0, map[string]any{"ok": true, "removed": name})
	}
}
