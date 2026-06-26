// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func init() {
	Auditors["wiring_invariant_auditor"] = AuditWiringInvariant
}

type wiringInvariant struct {
	relPath  string
	mustHave []string
	reason   string
}

var wiringInvariants = []wiringInvariant{

	{
		relPath:  "Documents/FLowork_os/router/internal/router/dispatcher.go",
		mustHave: []string{"maybeInjectAntibodies", "maybeReinforceAntibody", "acquireDispatchSlot"},
		reason:   "Hook antibody inject + feedback + bergantian/anti-429 — kalau ilang: halu balik / loop putus / fleet kena 429",
	},
	{
		relPath:  "Documents/FLowork_os/router/internal/router/ratelimit.go",
		mustHave: []string{"func backoffDuration", "claudeSem", "maxRateLimitRetries"},
		reason:   "Engine anti-429 (bergantian + backoff) — kalau ilang, armada agent nembak Claude barengan → 429 → task gagal",
	},
	{
		relPath:  "Documents/FLowork_os/router/internal/router/dispatcher_stream.go",
		mustHave: []string{"maybeInjectAntibodies"},
		reason:   "Hook antibody anti-halu (stream)",
	},
	{
		relPath:  "Documents/FLowork_os/router/internal/router/mistakeenrich.go",
		mustHave: []string{"func maybeInjectAntibodies", "rankAntibodies"},
		reason:   "Engine antibody injection — file inti anti-halu deterministik",
	},
	{
		relPath:  "Documents/FLowork_os/router/internal/router/mistakefeedback.go",
		mustHave: []string{"func maybeReinforceAntibody", "detectNonCanonicalTaskRun"},
		reason:   "Engine feedback loop — kalau ilang, antibody ga self-learning (karma ga naik dari halu)",
	},

	{
		relPath:  "Documents/FLowork_os/agent/agents/mr-flow/main.go",
		mustHave: []string{"deterministicRoute"},
		reason:   "Routing deterministik mr-flow — kalau ilang, dispatch balik ngandelin LLM lemah (rapuh)",
	},
}

var (
	wiInvMu   sync.Mutex
	wiInvLast time.Time
)

func AuditWiringInvariant(filePath, content string) []Finding {
	wiInvMu.Lock()
	if time.Since(wiInvLast) < 2*time.Second {
		wiInvMu.Unlock()
		return nil
	}
	wiInvLast = time.Now()
	wiInvMu.Unlock()

	base := invariantBase()
	if base == "" {

		return nil
	}
	return checkInvariants(wiringInvariants, func(rel string) (string, error) {
		b, e := os.ReadFile(filepath.Join(base, rel))
		return string(b), e
	})
}

func invariantBase() string {
	marker := wiringInvariants[0].relPath
	var cands []string
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		cands = append(cands, h)
	}

	for _, k := range []string{"USER", "LOGNAME"} {
		if u := strings.TrimSpace(os.Getenv(k)); u != "" {
			cands = append(cands, filepath.Join("/home", u), filepath.Join("/Users", u))
		}
	}

	if r := strings.TrimSpace(os.Getenv("FLOWORK_CODESCAN_ROOT")); r != "" {
		cands = append(cands, filepath.Dir(filepath.Dir(r)), filepath.Dir(filepath.Dir(filepath.Dir(r))))
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, marker)); err == nil {
			return c
		}
	}
	return ""
}

func checkInvariants(invs []wiringInvariant, read func(rel string) (string, error)) []Finding {
	var out []Finding
	for _, inv := range invs {
		s, err := read(inv.relPath)
		if err != nil {
			out = append(out, Finding{
				Auditor:     "wiring_invariant_auditor",
				Severity:    SevCritical,
				FilePath:    inv.relPath,
				LineNumber:  0,
				Message:     "WIRING HILANG: file pipa kritis ga kebaca/ga ada — " + inv.reason,
				Snippet:     inv.relPath,
				Remediation: "Restore file. Ini pipa kritis yang DILARANG dihapus AI tanpa izin Mr.Dev.",
			})
			continue
		}
		for _, must := range inv.mustHave {
			if !strings.Contains(s, must) {
				out = append(out, Finding{
					Auditor:     "wiring_invariant_auditor",
					Severity:    SevCritical,
					FilePath:    inv.relPath,
					LineNumber:  0,
					Message:     "WIRING PUTUS: '" + must + "' ILANG dari file — " + inv.reason,
					Snippet:     must,
					Remediation: "Sambungin lagi '" + must + "'. Pipa kritis, DILARANG diubah AI tanpa izin Mr.Dev (cek changelog + lock header).",
				})
			}
		}
	}
	return out
}
