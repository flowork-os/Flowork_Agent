// scan_parse.go — PARSER deterministik output scan tool → finding terstruktur (immune P2.2b + C).
//
// Prinsip #1 (KONTRAK): "agent BODOH, engine PINTER" → ZERO LLM nebak vuln.
// Parser ini murni deterministik (XML/JSON decode), bisa di-unit-test, reproducible.
//
// Dispatch per-tool by basename:
//   nmap   → XML  (-oX -)            → port terbuka (attack surface, info)
//   nuclei → JSONL (-jsonl)          → vuln/exposure (severity+CWE+CVE+CVSS bawaan)
//   trivy  → JSON  (fs --format json)→ CVE dependensi (supply-chain, severity+CVSS)
//
// Nambah tool = tambah 1 parser + 1 case. Tool tak-dikenal → nil (ga error, run
// tetep ke-audit di scan_runs; cuma ga ada finding terstruktur).

package scanapi

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"

	"flowork-gui/internal/floworkdb"
)

// parseFindings — dispatch parse by tool basename. Stamp tool/target/category +
// normalisasi severity. Deterministik, ga pernah panic (decode error → nil).
func parseFindings(tool, target, category, stdout string) []floworkdb.ScanFinding {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(tool), ".exe"))
	var fs []floworkdb.ScanFinding
	switch base {
	case "nmap":
		fs = parseNmapXML(stdout)
	case "nuclei":
		fs = parseNucleiJSONL(stdout)
	case "trivy":
		fs = parseTrivyJSON(stdout)
	case "subfinder":
		fs = parseSubfinderJSONL(stdout)
	case "dig":
		fs = parseDig(stdout)
	default:
		return nil
	}
	cat := normScanCategory(category)
	for i := range fs {
		fs[i].Tool = base
		if fs[i].Target == "" {
			fs[i].Target = target
		}
		fs[i].Category = cat
		fs[i].Severity = normSeverity(fs[i].Severity)
	}
	return fs
}

// normSeverity — peta severity vendor-beda → kanonik info|low|medium|high|critical.
func normSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return "critical"
	case "high", "important":
		return "high"
	case "medium", "moderate":
		return "medium"
	case "low", "minor":
		return "low"
	default:
		return "info"
	}
}

// normScanCategory — defensif default. Cuma "pentest" yang ofensif.
func normScanCategory(c string) string {
	if strings.ToLower(strings.TrimSpace(c)) == "pentest" {
		return "pentest"
	}
	return "immune"
}

// ── nmap XML ──────────────────────────────────────────────────────────────
// Port terbuka = finding info (attack surface). Service product/version → component.

type nmapService struct {
	Name    string `xml:"name,attr"`
	Product string `xml:"product,attr"`
	Version string `xml:"version,attr"`
}
type nmapState struct {
	State string `xml:"state,attr"`
}
type nmapPort struct {
	Protocol string      `xml:"protocol,attr"`
	PortID   string      `xml:"portid,attr"`
	State    nmapState   `xml:"state"`
	Service  nmapService `xml:"service"`
}
type nmapAddr struct {
	Addr string `xml:"addr,attr"`
	Type string `xml:"addrtype,attr"`
}
type nmapHost struct {
	Addresses []nmapAddr  `xml:"address"`
	Ports     []nmapPort  `xml:"ports>port"`
}
type nmapRun struct {
	Hosts []nmapHost `xml:"host"`
}

func parseNmapXML(out string) []floworkdb.ScanFinding {
	out = strings.TrimSpace(out)
	if !strings.Contains(out, "<nmaprun") {
		return nil
	}
	var run nmapRun
	if err := xml.Unmarshal([]byte(out), &run); err != nil {
		return nil
	}
	var fs []floworkdb.ScanFinding
	for _, h := range run.Hosts {
		addr := ""
		for _, a := range h.Addresses {
			if a.Type == "ipv4" || a.Type == "ipv6" || addr == "" {
				addr = a.Addr
			}
		}
		for _, p := range h.Ports {
			if p.State.State != "open" {
				continue
			}
			svc := p.Service.Name
			if svc == "" {
				svc = "unknown"
			}
			comp := svc
			if p.Service.Product != "" {
				comp = strings.TrimSpace(p.Service.Product + " " + p.Service.Version)
			}
			fs = append(fs, floworkdb.ScanFinding{
				Severity:  "info",
				Title:     fmt.Sprintf("Open port %s/%s (%s)", p.PortID, p.Protocol, svc),
				Target:    addr,
				Component: comp,
				Evidence:  fmt.Sprintf("%s:%s/%s open", addr, p.PortID, p.Protocol),
			})
		}
	}
	return fs
}

// ── nuclei JSONL ──────────────────────────────────────────────────────────
// 1 JSON per baris. severity+classification (CVE/CWE/CVSS) bawaan template.

type nucleiLine struct {
	TemplateID string `json:"template-id"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	MatchedAt  string `json:"matched-at"`
	Info       struct {
		Name           string `json:"name"`
		Severity       string `json:"severity"`
		Classification struct {
			CVEID     []string `json:"cve-id"`
			CWEID     []string `json:"cwe-id"`
			CVSSScore float64  `json:"cvss-score"`
		} `json:"classification"`
	} `json:"info"`
}

func parseNucleiJSONL(out string) []floworkdb.ScanFinding {
	var fs []floworkdb.ScanFinding
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r nucleiLine
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		title := r.Info.Name
		if title == "" {
			title = r.TemplateID
		}
		f := floworkdb.ScanFinding{
			Severity:  r.Info.Severity,
			Title:     title,
			Component: r.TemplateID,
			Evidence:  r.MatchedAt,
			CVSS:      r.Info.Classification.CVSSScore,
			Target:    r.Host,
		}
		if len(r.Info.Classification.CVEID) > 0 {
			f.CVE = strings.ToUpper(strings.Join(r.Info.Classification.CVEID, ","))
		}
		if len(r.Info.Classification.CWEID) > 0 {
			f.CWE = strings.ToUpper(r.Info.Classification.CWEID[0])
		}
		fs = append(fs, f)
	}
	return fs
}

// ── trivy JSON ────────────────────────────────────────────────────────────
// Results[].Vulnerabilities[] → CVE dependensi (supply-chain). CVSS V3 diutamakan.

type trivyCVSS struct {
	V3Score float64 `json:"V3Score"`
	V2Score float64 `json:"V2Score"`
}
type trivyVuln struct {
	VulnerabilityID  string               `json:"VulnerabilityID"`
	PkgName          string               `json:"PkgName"`
	InstalledVersion string               `json:"InstalledVersion"`
	Severity         string               `json:"Severity"`
	Title            string               `json:"Title"`
	CweIDs           []string             `json:"CweIDs"`
	CVSS             map[string]trivyCVSS `json:"CVSS"`
}
type trivyResult struct {
	Target          string      `json:"Target"`
	Vulnerabilities []trivyVuln `json:"Vulnerabilities"`
}
type trivyDoc struct {
	Results []trivyResult `json:"Results"`
}

func parseTrivyJSON(out string) []floworkdb.ScanFinding {
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "{") {
		return nil
	}
	var doc trivyDoc
	if json.Unmarshal([]byte(out), &doc) != nil {
		return nil
	}
	var fs []floworkdb.ScanFinding
	for _, res := range doc.Results {
		for _, v := range res.Vulnerabilities {
			title := v.Title
			if title == "" {
				title = fmt.Sprintf("%s in %s", v.VulnerabilityID, v.PkgName)
			}
			cwe := ""
			if len(v.CweIDs) > 0 {
				cwe = strings.ToUpper(v.CweIDs[0])
			}
			fs = append(fs, floworkdb.ScanFinding{
				Severity:  v.Severity,
				Title:     title,
				CVE:       v.VulnerabilityID,
				CWE:       cwe,
				CVSS:      pickCVSS(v.CVSS),
				Component: v.PkgName + "@" + v.InstalledVersion,
				Evidence:  res.Target,
			})
		}
	}
	return fs
}

// pickCVSS — pilih skor V3 (utamakan nvd→ghsa→redhat→apa aja), fallback V2.
func pickCVSS(m map[string]trivyCVSS) float64 {
	for _, pref := range []string{"nvd", "ghsa", "redhat"} {
		if c, ok := m[pref]; ok && c.V3Score > 0 {
			return c.V3Score
		}
	}
	for _, c := range m {
		if c.V3Score > 0 {
			return c.V3Score
		}
	}
	for _, c := range m {
		if c.V2Score > 0 {
			return c.V2Score
		}
	}
	return 0
}

// ── subfinder JSONL ───────────────────────────────────────────────────────
// 1 JSON per baris {host,input,source}. Subdomain = attack surface (info). Dedup.

func parseSubfinderJSONL(out string) []floworkdb.ScanFinding {
	var fs []floworkdb.ScanFinding
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r struct {
			Host   string `json:"host"`
			Input  string `json:"input"`
			Source string `json:"source"`
		}
		if json.Unmarshal([]byte(line), &r) != nil || r.Host == "" || seen[r.Host] {
			continue
		}
		seen[r.Host] = true
		fs = append(fs, floworkdb.ScanFinding{
			Severity:  "info",
			Title:     "Subdomain: " + r.Host,
			Target:    r.Input,
			Component: r.Source,
			Evidence:  r.Host,
		})
	}
	return fs
}

// ── dig answer section ────────────────────────────────────────────────────
// Baris `name TTL IN TYPE value` (output `dig +noall +answer`). Tiap record DNS
// = finding info (attack surface / recon). Skip komentar + non-IN.

func parseDig(out string) []floworkdb.ScanFinding {
	var fs []floworkdb.ScanFinding
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 || f[2] != "IN" {
			continue
		}
		typ := f[3]
		val := strings.Join(f[4:], " ")
		name := strings.TrimSuffix(f[0], ".")
		fs = append(fs, floworkdb.ScanFinding{
			Severity:  "info",
			Title:     "DNS " + typ + ": " + val,
			Target:    name,
			Component: typ,
			Evidence:  name + " " + typ + " " + val,
		})
	}
	return fs
}
