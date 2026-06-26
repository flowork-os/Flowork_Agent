// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

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

func ToolScan(target string) []Finding {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	if _, err := exec.LookPath("trivy"); err != nil {
		return nil
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
	_ = cmd.Run()
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

var immuneToolset = []string{"trivy"}

func ToolNames() []string {
	names := make([]string, 0, len(immuneToolset))
	for _, bin := range immuneToolset {
		if _, err := exec.LookPath(bin); err == nil {
			names = append(names, bin)
		}
	}
	return names
}

func IsDepManifest(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "yarn.lock",
		"requirements.txt", "pipfile.lock", "gemfile.lock", "pom.xml",
		"cargo.lock", "composer.lock", "pyproject.toml":
		return true
	}
	return false
}
