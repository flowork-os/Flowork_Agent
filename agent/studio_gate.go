// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN (gerbang AI Studio — stabil, security-sensitif). Dok: lock/ai-studio.md.
//
// studio_gate.go — GERBANG PEMERIKSAAN AI Studio per-jenis (ROADMAP_AI_STUDIO F2), CORE
// yang dibekuin biar AI/asisten masa depan ga ngubah logika gate yang udah stabil TANPA
// SADAR. Tetap EXTENSIBLE lewat 3 switch (Rule #7 — freeze WAJIB bikin switch dulu):
//
//	RegisterCapabilityVerifier(kind, fn) — POLA A: jenis kapabilitas BARU diperiksa lewat
//	    sibling init() tanpa nyentuh file ini.
//	var verifyAgentPack                  — POLA B: jalur AGENT (butuh manifest/persona/wasm)
//	    di-override verifier.go (non-frozen). Default = cek AMAN minimal (zip+plugin.json).
//	    Hapus verifier.go → fallback default, build tetap 0 (self-sufficient).
//
// REUSE scanner FROZEN adopt.ScanRepo (internal/apps/adopt/scan.go) — 1 sumber pola-jahat,
// dipakai bareng "adopt repo". App .fwpack (zip) → extract temp-dir → ScanRepo → cleanup.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"flowork-gui/internal/apps/adopt"
)

// VerifyCheck — 1 hasil cek. Status: "pass" | "warn" | "fail".
type VerifyCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`   // pass | warn | fail
	Severity string `json:"severity"` // info | medium | high | critical
	Detail   string `json:"detail"`
}

// VerifyVerdict — keputusan akhir gerbang.
//
//	"approved" → semua pass, aman go-live.
//	"review"   → ada warn (caps bahaya / consent / quality) → butuh mata owner.
//	"blocked"  → ada fail (struktur rusak / pola berbahaya) → JANGAN deploy.
type VerifyVerdict struct {
	Status  string        `json:"status"`
	Score   int           `json:"score"` // 0-100 (100 = bersih)
	Checks  []VerifyCheck `json:"checks"`
	Summary string        `json:"summary"`
}

// finalizeVerdict — hitung status + score dari checks. ada fail → blocked. ada warn →
// review. else → approved. score = 100 - (fail*40 + warn*15), clamp [0,100].
func finalizeVerdict(checks []VerifyCheck) VerifyVerdict {
	fails, warns := 0, 0
	for _, c := range checks {
		switch c.Status {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	score := 100 - (fails*40 + warns*15)
	if score < 0 {
		score = 0
	}
	status := "approved"
	summary := "pack clean — safe to go live"
	switch {
	case fails > 0:
		status = "blocked"
		summary = "pack BLOCKED — failing checks (structure/dangerous patterns), do NOT deploy"
	case warns > 0:
		status = "review"
		summary = "pack needs owner REVIEW — warnings (dangerous caps / consent / quality)"
	}
	return VerifyVerdict{Status: status, Score: score, Checks: checks, Summary: summary}
}

// gateIDRe — validasi id kapabilitas (self-contained: gate ga gantung simbol non-frozen).
var gateIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

// capabilityVerifiers — SWITCH (POLA A). Registry jenis-kapabilitas → fungsi periksa.
// Default KOSONG = aman (jatuh ke built-in/agent default). Sibling daftar lewat init().
var capabilityVerifiers = map[string]func([]byte) VerifyVerdict{}

// RegisterCapabilityVerifier — daftarin pemeriksa jenis BARU (mis. "workflow") via sibling
// init(). kind kosong / fn nil → di-skip (aman). Nimpa kind built-in (app/tool/...) kalau
// owner sengaja override.
func RegisterCapabilityVerifier(kind string, fn func([]byte) VerifyVerdict) {
	if strings.TrimSpace(kind) == "" || fn == nil {
		return
	}
	capabilityVerifiers[kind] = fn
}

// verifyAgentPack — SWITCH (POLA B). Jalur AGENT (kind kosong/agent/category) butuh cek
// kaya (manifest+persona+wasm) yang hidup di verifier.go (non-frozen). Default di sini =
// cek AMAN minimal biar gate self-sufficient kalau verifier.go dihapus.
var verifyAgentPack = func(raw []byte) VerifyVerdict {
	checks := []VerifyCheck{}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		checks = append(checks, VerifyCheck{"zip_valid", "fail", "critical", "not a valid zip: " + err.Error()})
		return finalizeVerdict(checks)
	}
	checks = append(checks, VerifyCheck{"zip_valid", "pass", "info", "valid zip archive"})
	if readPackManifest(zr) == nil {
		checks = append(checks, VerifyCheck{"manifest_present", "fail", "critical", "plugin.json missing from pack"})
	} else {
		checks = append(checks, VerifyCheck{"manifest_present", "warn", "medium", "default agent verifier (verifier.go absent) — owner review"})
	}
	return finalizeVerdict(checks)
}

// verifyCapability — 1 PINTU periksa, dispatch dari `kind` plugin.json:
//
//	registry (RegisterCapabilityVerifier) → kalau ada, dipakai DULUAN (kind baru/override).
//	"app"                      → verifyAppPack    (skip wasm; +scan pola-jahat +consent exec)
//	tool/slash/scanner/channel → verifyGenericPack (struktur zip + manifest + scan, no wasm)
//	"" / agent / category      → verifyAgentPack   (var: verifier.go override; default minimal)
func verifyCapability(kind string, raw []byte) VerifyVerdict {
	if fn, ok := capabilityVerifiers[kind]; ok {
		return fn(raw)
	}
	switch kind {
	case "app":
		return verifyAppPack(raw)
	case "tool", "slash", "scanner", "channel":
		return verifyGenericPack(raw, kind)
	default:
		return verifyAgentPack(raw)
	}
}

// readPackManifest — ambil plugin.json mentah dari zip (nil kalau ga ada).
func readPackManifest(zr *zip.Reader) []byte {
	for _, f := range zr.File {
		base := strings.TrimPrefix(f.Name, "./")
		if base == "plugin.json" || strings.HasSuffix(base, "/plugin.json") {
			rc, e := f.Open()
			if e != nil {
				return nil
			}
			raw, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			return raw
		}
	}
	return nil
}

// scanPackBytes — extract zip .fwpack ke temp-dir lalu adopt.ScanRepo (scanner FROZEN butuh
// folder). REUSE pola-jahat yang sama dengan "adopt repo". temp dihapus. Anti zip-slip.
func scanPackBytes(raw []byte) (adopt.ScanReport, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return adopt.ScanReport{}, err
	}
	tmp, err := os.MkdirTemp("", "fw-verify-scan-")
	if err != nil {
		return adopt.ScanReport{}, err
	}
	defer os.RemoveAll(tmp)
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		dest := filepath.Join(tmp, filepath.FromSlash(name))
		if rel, e := filepath.Rel(tmp, dest); e != nil || strings.HasPrefix(rel, "..") {
			continue // anti zip-slip
		}
		if e := os.MkdirAll(filepath.Dir(dest), 0o755); e != nil {
			continue
		}
		rc, e := f.Open()
		if e != nil {
			continue
		}
		out, e := os.Create(dest)
		if e != nil {
			rc.Close()
			continue
		}
		_, _ = io.Copy(out, io.LimitReader(rc, 16<<20))
		out.Close()
		rc.Close()
	}
	return adopt.ScanRepo(tmp), nil
}

// addDangerScanCheck — map ScanReport → VerifyCheck. critical → fail (blocked), warn → warn
// (review), bersih → pass.
func addDangerScanCheck(add func(name, status, sev, detail string), rep adopt.ScanReport) {
	switch {
	case rep.Critical > 0:
		add("danger_scan", "fail", "critical",
			fmt.Sprintf("%d pola BERBAHAYA (rm-rf/pipe-shell/reverse-shell/SSRF-metadata): %s",
				rep.Critical, summarizeFindings(rep, "critical")))
	case rep.Warn > 0:
		add("danger_scan", "warn", "medium",
			fmt.Sprintf("%d pola perlu-dicek: %s", rep.Warn, summarizeFindings(rep, "warn")))
	default:
		add("danger_scan", "pass", "info",
			fmt.Sprintf("no dangerous patterns (%d file di-scan)", rep.Scanned))
	}
}

// summarizeFindings — ringkas N finding sev tertentu (max 4, "file:line pattern").
func summarizeFindings(rep adopt.ScanReport, sev string) string {
	out := []string{}
	for _, f := range rep.Findings {
		if f.Severity != sev {
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d %s", f.File, f.Line, f.Pattern))
		if len(out) >= 4 {
			out = append(out, "…")
			break
		}
	}
	return strings.Join(out, "; ")
}

// verifyAppPack — periksa app .fwpack (kind:app). BUKAN agent: SKIP agent.wasm/persona/
// 1-synth. Cek: zip → plugin.json (kind=app,id) → consent exec (app = perintah OS) →
// scan pola-jahat di core.*/ui/* (REUSE).
func verifyAppPack(raw []byte) VerifyVerdict {
	checks := []VerifyCheck{}
	add := func(name, status, sev, detail string) {
		checks = append(checks, VerifyCheck{Name: name, Status: status, Severity: sev, Detail: detail})
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		add("zip_valid", "fail", "critical", "not a valid zip: "+err.Error())
		return finalizeVerdict(checks)
	}
	add("zip_valid", "pass", "info", "valid zip archive")

	manRaw := readPackManifest(zr)
	if manRaw == nil {
		add("manifest_present", "fail", "critical", "plugin.json missing from pack")
		return finalizeVerdict(checks)
	}
	var meta struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if json.Unmarshal(manRaw, &meta) != nil {
		add("manifest_parse", "fail", "critical", "plugin.json parse error")
		return finalizeVerdict(checks)
	}
	if meta.Kind != "app" || !gateIDRe.MatchString(meta.ID) {
		add("manifest_structure", "fail", "high", "app manifest invalid (kind WAJIB 'app', id valid)")
	} else {
		add("manifest_structure", "pass", "info", "kind=app, id valid")
	}

	// App jalanin core_entry (perintah OS) → consent exec owner WAJIB. App bersih = "review",
	// BUKAN "approved" — jujur (owner buka gerbang exec).
	add("exec_consent", "warn", "medium",
		"app menjalankan core_entry (perintah OS) — consent exec owner WAJIB (approve_exec)")

	if rep, e := scanPackBytes(raw); e == nil {
		addDangerScanCheck(add, rep)
	} else {
		add("danger_scan", "warn", "medium", "scan skipped: "+e.Error())
	}
	return finalizeVerdict(checks)
}

// verifyGenericPack — periksa pack non-agent non-app (tool/slash/scanner/channel). Payload
// tiap jenis beda → cek MINIMAL umum: zip + plugin.json + scan pola-jahat. TANPA maksa wasm.
func verifyGenericPack(raw []byte, kind string) VerifyVerdict {
	checks := []VerifyCheck{}
	add := func(name, status, sev, detail string) {
		checks = append(checks, VerifyCheck{Name: name, Status: status, Severity: sev, Detail: detail})
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		add("zip_valid", "fail", "critical", "not a valid zip: "+err.Error())
		return finalizeVerdict(checks)
	}
	add("zip_valid", "pass", "info", "valid zip archive")
	if readPackManifest(zr) == nil {
		add("manifest_present", "fail", "critical", "plugin.json missing from pack")
		return finalizeVerdict(checks)
	}
	add("manifest_present", "pass", "info", "plugin.json present (kind="+kind+")")
	if rep, e := scanPackBytes(raw); e == nil {
		addDangerScanCheck(add, rep)
	} else {
		add("danger_scan", "warn", "medium", "scan skipped: "+e.Error())
	}
	return finalizeVerdict(checks)
}
