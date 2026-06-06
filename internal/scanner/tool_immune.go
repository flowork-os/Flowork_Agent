// tool_immune.go — IMUN lewat TOOL NYATA (trivy), nyatu ke pipeline auditor.
//
// 115 auditor statis = pattern-match (regex) di kode sendiri. Ini NAMBAHIN tool
// nyata: trivy fs → CVE dependensi (DB CVE beneran) + secret + IaC misconfig —
// hal yang pattern-auditor ga bisa. Output = []Finding (FORMAT SAMA auditor) →
// codescan engine nyimpennya lewat jalur yang SAMA → muncul di Scan Log + Findings
// + baseline Threat Radar, persis kayak auditor.
//
// Kode SENDIRI = selalu authorized (defensif) → NO gerbang allowlist target.
// trivy ga kepasang → nil (imun tetep jalan pakai auditor). Bounded + graceful.

package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolScan — scan `target` (repo root atau file manifest) pakai trivy. Return
// []Finding (kosong/nil kalau bersih / trivy ga ada / error). Aman dipanggil
// dari engine codescan: ga panic, ada timeout.
func ToolScan(target string) []Finding {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	if _, err := exec.LookPath("trivy"); err != nil {
		return nil // trivy ga kepasang → imun tetep jalan pakai auditor statis
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "trivy", "fs",
		"--scanners", "vuln,secret,misconfig",
		"--format", "json", "--quiet",
		"--skip-dirs", "vendor,node_modules,.git,bin,.scratch",
		target)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run() // trivy exit != 0 kalau nemu temuan — output JSON tetep valid
	if out.Len() == 0 {
		return nil
	}
	return parseTrivyImmune(out.Bytes())
}

type trivyImmuneDoc struct {
	Results []struct {
		Target          string `json:"Target"`
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			Severity         string `json:"Severity"`
			Title            string `json:"Title"`
		} `json:"Vulnerabilities"`
		Secrets []struct {
			RuleID    string `json:"RuleID"`
			Severity  string `json:"Severity"`
			Title     string `json:"Title"`
			StartLine int    `json:"StartLine"`
		} `json:"Secrets"`
		Misconfigurations []struct {
			ID         string `json:"ID"`
			Title      string `json:"Title"`
			Severity   string `json:"Severity"`
			Resolution string `json:"Resolution"`
		} `json:"Misconfigurations"`
	} `json:"Results"`
}

func parseTrivyImmune(raw []byte) []Finding {
	var doc trivyImmuneDoc
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var fs []Finding
	for _, r := range doc.Results {
		for _, v := range r.Vulnerabilities {
			rem := ""
			if v.FixedVersion != "" {
				rem = "update " + v.PkgName + " → " + v.FixedVersion
			}
			fs = append(fs, Finding{
				Auditor:     "trivy_dep",
				Severity:    trivySev(v.Severity),
				FilePath:    r.Target,
				Message:     fmt.Sprintf("%s in %s@%s — %s", v.VulnerabilityID, v.PkgName, v.InstalledVersion, v.Title),
				Remediation: rem,
			})
		}
		for _, s := range r.Secrets {
			fs = append(fs, Finding{
				Auditor:     "trivy_secret",
				Severity:    trivySev(s.Severity),
				FilePath:    r.Target,
				LineNumber:  s.StartLine,
				Message:     fmt.Sprintf("hardcoded secret: %s (%s)", s.Title, s.RuleID),
				Remediation: "pindahin secret ke env / secret-store, rotate kalau pernah bocor",
			})
		}
		for _, m := range r.Misconfigurations {
			fs = append(fs, Finding{
				Auditor:     "trivy_misconfig",
				Severity:    trivySev(m.Severity),
				FilePath:    r.Target,
				Message:     fmt.Sprintf("%s: %s", m.ID, m.Title),
				Remediation: m.Resolution,
			})
		}
	}
	return fs
}

func trivySev(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CRITICAL":
		return SevCritical
	case "HIGH":
		return SevHigh
	case "MEDIUM":
		return SevMedium
	case "LOW":
		return SevLow
	default:
		return SevInfo
	}
}

// immuneToolset — tool NYATA yang nambah deteksi imun AUTO-scan (jalan sendiri
// tiap kode/dependensi berubah). Plug-and-play seam: tambah binari di sini →
// OTOMATIS keitung sebagai scanner aktif KALAU binari-nya beneran kepasang.
var immuneToolset = []string{"trivy"}

// ToolNames — tool imun yang BENERAN kepasang (auto-detect via PATH). Dipakai
// buat hitung "scanner aktif" → angka ngikut realita: pasang tool → naik, copot
// → turun. BUKAN angka hardcode. Tool on-demand (nmap/nuclei/subfinder/dig)
// SENGAJA ga diitung di sini — dia owner-gated manual, muncul di scan log pas
// dijalanin, bukan auto-scan latar.
func ToolNames() []string {
	names := make([]string, 0, len(immuneToolset))
	for _, bin := range immuneToolset {
		if _, err := exec.LookPath(bin); err == nil {
			names = append(names, bin)
		}
	}
	return names
}

// IsDepManifest — file = manifest dependensi? Kalau berubah → re-scan trivy
// (CVE supply-chain). File kode biasa cukup auditor statis.
func IsDepManifest(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "yarn.lock",
		"requirements.txt", "pipfile.lock", "gemfile.lock", "pom.xml",
		"cargo.lock", "composer.lock", "pyproject.toml":
		return true
	}
	return false
}
