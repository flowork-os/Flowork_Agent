// scan_parse_test.go — unit test parser deterministik. Fixture = SCHEMA NYATA
// hasil capture tool sungguhan (nmap localhost, trivy deps vuln, nuclei JSONL).
// Reproducible, ga butuh tool kepasang → CI-friendly.

package scanapi

import (
	"strings"
	"testing"
)

// fixture nmap: STRUKTUR NYATA dari `nmap -oX -` (3 port open + 1 closed buat cek filter).
const fxNmap = `<?xml version="1.0"?><nmaprun scanner="nmap">
<host><status state="up"/><address addr="127.0.0.1" addrtype="ipv4"/>
<ports>
<port protocol="tcp" portid="631"><state state="open" reason="syn-ack"/><service name="ipp" method="table"/></port>
<port protocol="tcp" portid="3000"><state state="open"/><service name="ppp" method="table"/></port>
<port protocol="tcp" portid="8081"><state state="open"/><service name="http" product="nginx" version="1.24"/></port>
<port protocol="tcp" portid="22"><state state="closed"/><service name="ssh"/></port>
</ports></host></nmaprun>`

// fixture trivy: ENTRY NYATA dari `trivy fs --format json` (CVE-2019-14234 Django).
const fxTrivy = `{"Results":[{"Target":"requirements.txt","Vulnerabilities":[
{"VulnerabilityID":"CVE-2019-14234","PkgName":"Django","InstalledVersion":"2.2.0","Severity":"CRITICAL",
"Title":"Django: SQL injection possibility in key and index lookups","CweIDs":["CWE-89"],
"CVSS":{"nvd":{"V3Score":9.8}}}]}]}`

// fixture nuclei: 1 baris JSONL format dokumentasi nuclei (-jsonl).
const fxNuclei = `{"template-id":"CVE-2021-44228","type":"http","host":"https://target.example.com",` +
	`"matched-at":"https://target.example.com/api","info":{"name":"Apache Log4j RCE",` +
	`"severity":"critical","classification":{"cve-id":["CVE-2021-44228"],"cwe-id":["cwe-502"],"cvss-score":10.0}}}
not-json-noise-line
{"template-id":"tech","type":"http","host":"https://t","info":{"name":"Tech","severity":"info"}}`

func TestParseNmap(t *testing.T) {
	fs := parseNmapXML(fxNmap)
	if len(fs) != 3 {
		t.Fatalf("nmap: want 3 open-port findings, got %d", len(fs))
	}
	for _, f := range fs {
		if f.Severity != "info" {
			t.Errorf("nmap port severity want info, got %q", f.Severity)
		}
		if f.Target != "127.0.0.1" {
			t.Errorf("nmap target want 127.0.0.1, got %q", f.Target)
		}
	}
	// port 8081 punya product → component "nginx 1.24".
	var got8081 string
	for _, f := range fs {
		if f.Component == "nginx 1.24" {
			got8081 = f.Title
		}
	}
	if got8081 == "" {
		t.Errorf("nmap: port 8081 product (nginx 1.24) not captured in component")
	}
}

func TestParseTrivy(t *testing.T) {
	fs := parseTrivyJSON(fxTrivy)
	if len(fs) != 1 {
		t.Fatalf("trivy: want 1 vuln, got %d", len(fs))
	}
	f := fs[0]
	if f.CVE != "CVE-2019-14234" {
		t.Errorf("trivy CVE want CVE-2019-14234, got %q", f.CVE)
	}
	if f.CWE != "CWE-89" {
		t.Errorf("trivy CWE want CWE-89, got %q", f.CWE)
	}
	if normSeverity(f.Severity) != "critical" {
		t.Errorf("trivy severity want critical, got %q", f.Severity)
	}
	if f.CVSS != 9.8 {
		t.Errorf("trivy CVSS want 9.8, got %v", f.CVSS)
	}
	if f.Component != "Django@2.2.0" {
		t.Errorf("trivy component want Django@2.2.0, got %q", f.Component)
	}
}

func TestParseNuclei(t *testing.T) {
	fs := parseNucleiJSONL(fxNuclei)
	if len(fs) != 2 {
		t.Fatalf("nuclei: want 2 findings (noise line skipped), got %d", len(fs))
	}
	f := fs[0]
	if f.Title != "Apache Log4j RCE" {
		t.Errorf("nuclei title want 'Apache Log4j RCE', got %q", f.Title)
	}
	if f.CVE != "CVE-2021-44228" {
		t.Errorf("nuclei CVE want CVE-2021-44228, got %q", f.CVE)
	}
	if f.CWE != "CWE-502" { // di-uppercase dari cwe-502
		t.Errorf("nuclei CWE want CWE-502, got %q", f.CWE)
	}
	if f.CVSS != 10.0 {
		t.Errorf("nuclei CVSS want 10.0, got %v", f.CVSS)
	}
	if f.Severity != "critical" {
		t.Errorf("nuclei severity want critical, got %q", f.Severity)
	}
}

// fixture subfinder: format NYATA `subfinder -json` (+ 1 duplikat buat cek dedup + noise).
const fxSubfinder = `{"host":"dl-engine.floworkos.com","input":"floworkos.com","source":"thc"}
{"host":"extension.floworkos.com","input":"floworkos.com","source":"crtsh"}
{"host":"dl-engine.floworkos.com","input":"floworkos.com","source":"dup"}
garbage-line
{"host":"api.floworkos.com","input":"floworkos.com","source":"dnsx"}`

// fixture dig: output NYATA `dig +noall +answer` (+ komentar + baris pendek di-skip).
const fxDig = `; <<>> DiG 9.18 <<>> floworkos.com
floworkos.com.		241	IN	A	104.21.43.81
floworkos.com.		241	IN	MX	22 route1.mx.cloudflare.net.
shortline here`

func TestParseSubfinder(t *testing.T) {
	fs := parseSubfinderJSONL(fxSubfinder)
	if len(fs) != 3 { // dl-engine dedup → 3 unik (dl-engine, extension, api)
		t.Fatalf("subfinder: want 3 unique subdomains, got %d", len(fs))
	}
	for _, f := range fs {
		if f.Severity != "info" || !strings.Contains(f.Title, "Subdomain:") {
			t.Errorf("subfinder finding malformed: %+v", f)
		}
		if f.Target != "floworkos.com" {
			t.Errorf("subfinder target want floworkos.com, got %q", f.Target)
		}
	}
}

func TestParseDig(t *testing.T) {
	fs := parseDig(fxDig)
	if len(fs) != 2 { // A + MX (komentar & shortline di-skip)
		t.Fatalf("dig: want 2 records, got %d", len(fs))
	}
	var hasA, hasMX bool
	for _, f := range fs {
		if f.Component == "A" && strings.Contains(f.Title, "104.21.43.81") {
			hasA = true
		}
		if f.Component == "MX" {
			hasMX = true
		}
		if f.Target != "floworkos.com" {
			t.Errorf("dig target want floworkos.com, got %q", f.Target)
		}
	}
	if !hasA || !hasMX {
		t.Errorf("dig: missing A(%v) or MX(%v)", hasA, hasMX)
	}
}

func TestParseFindingsDispatch(t *testing.T) {
	// dispatch stamp tool+category+severity-norm.
	fs := parseFindings("/usr/bin/nmap", "127.0.0.1", "pentest", fxNmap)
	if len(fs) != 3 {
		t.Fatalf("dispatch nmap: want 3, got %d", len(fs))
	}
	for _, f := range fs {
		if f.Tool != "nmap" {
			t.Errorf("dispatch tool want nmap, got %q", f.Tool)
		}
		if f.Category != "pentest" {
			t.Errorf("dispatch category want pentest, got %q", f.Category)
		}
	}
	// tool tak-dikenal → nil (ga error).
	if parseFindings("whatweb", "x", "immune", "blah") != nil {
		t.Errorf("unknown tool should yield nil")
	}
	// category default → immune.
	fi := parseFindings("trivy", "p", "", fxTrivy)
	if len(fi) != 1 || fi[0].Category != "immune" {
		t.Errorf("default category want immune, got %+v", fi)
	}
}

func TestNormSeverity(t *testing.T) {
	cases := map[string]string{
		"CRITICAL": "critical", "High": "high", "MEDIUM": "medium", "moderate": "medium",
		"low": "low", "informational": "info", "unknown": "info", "": "info", "weird": "info",
	}
	for in, want := range cases {
		if got := normSeverity(in); got != want {
			t.Errorf("normSeverity(%q)=%q want %q", in, got, want)
		}
	}
}
