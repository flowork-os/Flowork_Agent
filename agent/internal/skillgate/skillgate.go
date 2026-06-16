// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-17
// Reason: P2 A2 fase-2a gerbang #2 (content verifier untrusted skill), owner-approved.
//   Tested: unit (danger/injection/no-false-positive) + integration handler path.
//   Ini anti-poison boundary — WAJIB sinkron dgn skill_author.go gate. Perketat =
//   perketat dua-duanya. Future change → tambah file/function baru atau izin owner.
//
// Package skillgate — CONTENT verifier untuk skill UNTRUSTED (P2 A2 fase-2a, gerbang #2).
//
// PURPOSE:
//   Skill (.fwskill) yang di-import dari luar = teks yang nanti MASUK PROMPT model
//   tiap turn (router inject by-keyword). Body skill = vektor poisoning (prompt
//   injection) + bisa nyuruh agent jalanin syscall berbahaya. Fase-1 import
//   (skills_exchange.go) cuma jamin STRUKTUR (anti-traversal, size, frontmatter) dan
//   sengaja nunda vetting NIAT konten ke owner. Gerbang ini meng-OTOMATIS-kan
//   vetting konten itu: scan body skill untuk pola berbahaya + injection sebelum
//   skill boleh ditulis ke disk.
//
// SUMBER KEBENARAN GATE (anti-poison boundary):
//   Pola di sini MIRROR `skillDangerRe`/`skillInjectRe` di
//   agent/internal/tools/builtins/skill_author.go (LOCKED — gate self-author).
//   Keduanya WAJIB sinkron: kalau salah satu di-perketat, perketat dua-duanya.
//   skillgate jadi paket reusable (import-path, registry-pull nanti) tanpa nyentuh
//   file LOCKED itu. Untuk skill UNTRUSTED dari luar, skillgate boleh LEBIH ketat
//   (superset) — gak boleh lebih longgar.
//
// Pure-Go, no deps, no side-effects (deterministik) — aman dipanggil di hot path import.
package skillgate

import (
	"regexp"
	"strings"
)

// dangerRe — pola syscall/exfil berbahaya (mirror skill_author.skillDangerRe).
// Match pola PERINTAH berbahaya, bukan kata Inggris biasa (skill yg mendeskripsikan
// kontrol komputer sah menyebut "shutdown"/"reboot"; eksekusi power di-gate terpisah).
var dangerRe = regexp.MustCompile(`(?i)(\brm\s+-rf|\bmkfs\b|:\(\)\s*\{|\bdd\s+if=|\bchmod\s+\+?s\b|\bsetuid\b|/etc/(passwd|shadow)|169\.254\.169\.254|\bcurl\s+[^|]*\|\s*(sh|bash)|\bwget\s+[^|]*\|\s*(sh|bash)|\bbase64\s+-d[^|]*\|\s*(sh|bash))`)

// injectRe — frasa prompt-injection/jailbreak (mirror skill_author.skillInjectRe).
// Skill = data yang model baca tiap turn → injection yang dibaked = vektor poisoning.
var injectRe = regexp.MustCompile(`(?i)(ignore\s+(all\s+)?previous|disregard\s+(all\s+)?(previous\s+)?instructions|reveal\s+(your\s+)?(system\s+)?prompt|abaikan\s+(instruksi|perintah)\s+sebelum|bocorkan\s+system\s+prompt|developer\s+mode|do\s+anything\s+now)`)

// Verify men-scan body skill dan mengembalikan daftar alasan ia TIDAK aman.
// Slice kosong = bersih (boleh disimpan). Dipanggil SEBELUM skill ditulis ke disk.
func Verify(content string) []string {
	var flags []string
	seen := map[string]bool{}
	for _, m := range dangerRe.FindAllString(content, -1) {
		key := "dangerous: " + strings.TrimSpace(m)
		if !seen[key] {
			seen[key] = true
			flags = append(flags, key)
		}
	}
	if m := injectRe.FindString(content); m != "" {
		flags = append(flags, "injection: "+strings.TrimSpace(m))
	}
	return flags
}

// Safe = true kalau konten lolos gate (tidak ada flag).
func Safe(content string) bool { return len(Verify(content)) == 0 }
