// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without explicit owner (Mr.Dev) approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-03
//
// ⚠️⚠️ PERINGATAN UNTUK AI MANAPUN (TERMASUK CLAUDE PASCA-COMPACT) ⚠️⚠️
//   File ini ADALAH penjaga yang nyegah "AI suka rubah-rubah jalur".
//   JANGAN hapus / lemahin / kurangin entri wiringInvariants tanpa Mr.Dev
//   minta EKSPLISIT. Kalau lo ngerasa pengen "ngerapihin" atau mindahin
//   pipa kritis → lo SALAH, itu persis hal yang dilarang. STOP, tanya dulu.
//   Nambah invariant baru = boleh (makin ketat makin bagus). Ngurangin = NO.
//
// auditors_invariant.go — WIRING INVARIANT GUARD.
//
// MASALAH YANG DIPECAHIN (kata Mr.Dev): "loe sendiri suka rubah2 jalur".
//   Lock-comment itu PASIF — AI yang ilang konteks bisa ngabaikan. Ini AKTIF:
//   scanner auto-jalan tiap file berubah + startup, jadi begitu ADA pipa
//   kritis yang dicabut/dirusak (siapa pun, termasuk AI pasca-compact) →
//   langsung muncul CRITICAL di Threat Radar. Enforcement yang SURVIVE amnesia.
//
// CARA KERJA: registry deklaratif {file, pola-wajib, alasan}. Tiap scan baca
//   file (DUA repo: Flowork_Agent + flowork_Router via path absolut home-relative),
//   cek SEMUA pola masih ada. Hilang/ilang file → CRITICAL. Fails-open kalau
//   home ga ke-resolve (jangan bikin scan crash).
//
// Debounce: cek lintas-repo jalan SEKALI per burst scan (anti duplikat N-file).

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

// wiringInvariant — satu pipa kritis yang DILARANG putus.
type wiringInvariant struct {
	relPath  string   // path relatif ke home (~), portable
	mustHave []string // SEMUA substring ini WAJIB ada di file
	reason   string   // kenapa kritis (muncul di finding)
}

// wiringInvariants — REGISTRY pipa kritis. Mr.Dev yang nambah; AI DILARANG
// ngurangin. Tiap entri = janji "pipa ini ga boleh putus".
var wiringInvariants = []wiringInvariant{
	// — Anti-halu: antibody injection (flowork_Router) —
	{
		relPath:  "Documents/flowork_Router/internal/router/dispatcher.go",
		mustHave: []string{"maybeInjectAntibodies", "maybeReinforceAntibody", "acquireDispatchSlot"},
		reason:   "Hook antibody inject + feedback + bergantian/anti-429 — kalau ilang: halu balik / loop putus / fleet kena 429",
	},
	{
		relPath:  "Documents/flowork_Router/internal/router/ratelimit.go",
		mustHave: []string{"func backoffDuration", "claudeSem", "maxRateLimitRetries"},
		reason:   "Engine anti-429 (bergantian + backoff) — kalau ilang, armada agent nembak Claude barengan → 429 → task gagal",
	},
	{
		relPath:  "Documents/flowork_Router/internal/router/dispatcher_stream.go",
		mustHave: []string{"maybeInjectAntibodies"},
		reason:   "Hook antibody anti-halu (stream)",
	},
	{
		relPath:  "Documents/flowork_Router/internal/router/mistakeenrich.go",
		mustHave: []string{"func maybeInjectAntibodies", "rankAntibodies"},
		reason:   "Engine antibody injection — file inti anti-halu deterministik",
	},
	{
		relPath:  "Documents/flowork_Router/internal/router/mistakefeedback.go",
		mustHave: []string{"func maybeReinforceAntibody", "detectNonCanonicalTaskRun"},
		reason:   "Engine feedback loop — kalau ilang, antibody ga self-learning (karma ga naik dari halu)",
	},
	// — Routing deterministik (Flowork_Agent / mr-flow) —
	{
		relPath:  "Documents/Flowork_Agent/agents/mr-flow/main.go",
		mustHave: []string{"deterministicRoute"},
		reason:   "Routing deterministik mr-flow — kalau ilang, dispatch balik ngandelin LLM lemah (rapuh)",
	},
}

// debounce supaya cek lintas-repo (yang baca absolute path sendiri) ga jalan
// berulang tiap file dalam satu scan.
var (
	wiInvMu   sync.Mutex
	wiInvLast time.Time
)

// AuditWiringInvariant — registered auditor. Jalan sekali per burst scan, cek
// semua wiringInvariants. Signature per-file (filePath/content) diabaikan —
// guard ini baca file kritis sendiri via path absolut (lintas-repo).
func AuditWiringInvariant(filePath, content string) []Finding {
	wiInvMu.Lock()
	if time.Since(wiInvLast) < 2*time.Second {
		wiInvMu.Unlock()
		return nil // udah dicek di burst scan ini
	}
	wiInvLast = time.Now()
	wiInvMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return nil // fails-open: jangan bikin scan crash
	}
	return checkInvariants(wiringInvariants, func(rel string) (string, error) {
		b, e := os.ReadFile(filepath.Join(home, rel))
		return string(b), e
	})
}

// checkInvariants — PURE (read di-inject) biar unit-testable tanpa filesystem.
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
