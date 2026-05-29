// Package safety — patterns.go: hardcoded blacklist patterns untuk Host
// Protection Gate (HPG) Phase 0 Day 0.
//
// Per KEPUTUSAN_FINAL.MD §1 + Keputusan 1/7 Antigravity/Opus-3:
// "FloworkOS JANGAN PERNAH bisa install virus/malware atau hack PC yang dia
// jalan di atasnya. Ini BUKAN hanya discipline rule — ini ARSITEKTUR HARD GATE."
//
// Pattern di file ini HARD-CODED const (immutable saat compile). BUKAN settings
// DB (anti DB tampering attack). BUKAN constitution (bisa di-jailbreak). HARD
// GATE = compile-time immutable.
//
// Edit pattern = recompile + redeploy semua node. Intentional friction.
//
// 4 kategori pattern:
//   1. DangerousSyscallPatterns — destructive command pattern (rm -rf, format, dll)
//   2. ProtectedSystemPaths     — write ke path system reserved
//   3. ProtectedNetworkTargets  — network call ke localhost / metadata IP
//   4. PrivilegeEscalationPatterns — runas / sudo bypass attempt

package safety

import (
	"regexp"
	"strings"
)

// ─── 1. Dangerous Syscall Patterns ──────────────────────────────────────────
//
// Match command string (case-insensitive) yang execute destructive action.
// Caller (HPG.Check) scan args yang berisi command string (bash, exec, etc).

var DangerousSyscallPatterns = []*regexp.Regexp{
	// Filesystem destruction
	// Pattern: rm dengan flag (recursive/force) + path destructive (root, ~, $HOME, C:\)
	// Match `rm -rf /` di mana pun (start of string, after =, dalam quote)
	regexp.MustCompile(`(?i)\brm\s+(-\S+\s+)+/`),                        // rm -rf /, rm -rf /home, etc (require flag)
	regexp.MustCompile(`(?i)\brm\s+(-\S+\s+)+\*`),                       // rm -rf * (current dir all)
	regexp.MustCompile(`(?i)\brm\s+(-\S+\s+)+~`),                        // rm -rf ~ (home)
	regexp.MustCompile(`(?i)\brm\s+(-\S+\s+)+\$HOME`),                   // rm -rf $HOME
	regexp.MustCompile(`(?i)\brm\s+(-\S+\s+)+[a-z]:[\\/]`),              // rm -rf C:\
	regexp.MustCompile(`(?i)\bdel\s+/[fsq]+`),                           // del /f /s /q (Windows)
	regexp.MustCompile(`(?i)\brmdir\s+/s`),                              // rmdir /s (Windows)
	regexp.MustCompile(`(?i)\bformat\s+[a-z]:`),                         // format C:
	regexp.MustCompile(`(?i)\bfdisk\b`),                                 // fdisk
	regexp.MustCompile(`(?i)\bmkfs\.\w+\s+/dev/`),                       // mkfs.ext4 /dev/sda
	regexp.MustCompile(`(?i)\bdd\s+if=\S+\s+of=/dev/`),                  // dd if=X of=/dev/sda
	regexp.MustCompile(`(?i)\bshred\s+`),                                // shred -u file
	regexp.MustCompile(`(?i)\bwipefs\b`),                                // wipefs

	// Process / system kill
	regexp.MustCompile(`(?i)\bkillall\s+-9`),                            // killall -9 (semua proses)
	regexp.MustCompile(`(?i)\bshutdown\s+-[hr]\s+now`),                  // shutdown -h now
	regexp.MustCompile(`(?i)\binit\s+0\b`),                              // init 0
	regexp.MustCompile(`(?i)\bsystemctl\s+poweroff\b`),                  // systemctl poweroff
	regexp.MustCompile(`(?i)(^|\s)halt(\s|$)`),                          // halt (alone, not part of word)

	// Registry destruction (Windows)
	regexp.MustCompile(`(?i)\breg\s+delete\s+`),                         // reg delete
	regexp.MustCompile(`(?i)\bremove-item\s+.*-recurse.*hklm`),          // PowerShell Remove-Item -Recurse HKLM:

	// Firewall disable
	regexp.MustCompile(`(?i)\bnetsh\s+(advfirewall\s+)?firewall\s+set\b.*(off|disable|state\s+off)`), // netsh firewall off
	regexp.MustCompile(`(?i)\bufw\s+disable\b`),                         // ufw disable (Linux)
	regexp.MustCompile(`(?i)\biptables\s+-F\b`),                         // iptables flush all
	regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable)\s+firewalld\b`), // systemctl stop firewalld

	// Cron / scheduled task hijack
	regexp.MustCompile(`(?i)\bcrontab\s+-r\b`),                          // crontab -r (delete all)
	regexp.MustCompile(`(?i)\bschtasks\s+/create.*\bSYSTEM\b`),          // schtasks create as SYSTEM

	// Network exfil pattern
	regexp.MustCompile(`(?i)\bcurl\s+[^|]+\|\s*(bash|sh|python|perl)\b`), // curl ... | bash
	regexp.MustCompile(`(?i)\bwget\s+[^|]+\|\s*(bash|sh|python|perl)\b`), // wget ... | sh
	regexp.MustCompile(`(?i)\bbash\s+<\(curl`),                          // bash <(curl ...)
	regexp.MustCompile(`(?i)\bnc\s+-l(p|vp|vlp)\s+\d+`),                 // nc -lvp listener

	// Reverse shell pattern
	regexp.MustCompile(`(?i)/dev/tcp/\d+\.\d+\.\d+\.\d+/\d+`),           // /dev/tcp/x/y reverse shell
	regexp.MustCompile(`(?i)\bbash\s+-i\s+>&\s*/dev/tcp/`),              // bash -i >& /dev/tcp/...
	regexp.MustCompile(`(?i)\bpython\s+-c\s+["'].*import\s+socket.*connect`), // python reverse shell

	// Self-modify kernel binary attempt (anti AI rogue)
	regexp.MustCompile(`(?i)\b(rm|del|chmod\s+000)\s+\S*kernel\.exe`),
	regexp.MustCompile(`(?i)\b(rm|del|chmod\s+000)\s+\S*flowork-(kernel|worker)`),
	regexp.MustCompile(`(?i)\bchattr\s+\+i\s+`),                         // immutable lock untuk hide
}

// ─── 2. Protected System Paths ──────────────────────────────────────────────
//
// Write ke paths ini = REJECT. Match by prefix (case-insensitive).
//
// Coverage: Linux (/etc, /usr, /sbin, /boot, /sys, /proc), macOS (/System,
// /Library/Apple), Windows (C:\Windows, C:\Program Files, C:\ProgramData),
// Android (/system, /vendor).

var ProtectedSystemPaths = []string{
	// Linux
	"/etc/",
	"/usr/bin/",
	"/usr/sbin/",
	"/usr/lib/",
	"/usr/local/bin/",
	"/sbin/",
	"/bin/",
	"/boot/",
	"/sys/",
	"/proc/",
	"/dev/sda",
	"/dev/nvme",
	"/dev/disk",
	"/var/lib/dpkg/",
	"/var/lib/rpm/",
	"/var/log/", // anti audit trail tampering
	"/root/",

	// macOS specific
	"/System/",
	"/Library/Apple/",
	"/private/etc/",
	"/private/var/db/",

	// Windows (case-insensitive matched di logic)
	"C:\\Windows\\",
	"C:/Windows/",
	"C:\\Program Files\\",
	"C:/Program Files/",
	"C:\\Program Files (x86)\\",
	"C:/Program Files (x86)/",
	"C:\\ProgramData\\",
	"C:/ProgramData/",

	// Android
	"/system/",
	"/vendor/",
	"/sbin/",
	"/data/system/",
	"/data/local/tmp/", // Termux escape vector

	// Flowork self-protection (anti AI rogue self-modify)
	".flowork/license.jwt",
	".flowork/master_key",
	".flowork/secrets/",
}

// ─── 3. Protected Network Targets ───────────────────────────────────────────
//
// Network call (curl, wget, nc, http.Get, etc) ke target ini = REJECT.
// Coverage: localhost (avoid SSRF ke kernel itself), cloud metadata,
// link-local (avoid AWS IMDS / GCP metadata pivot).

var ProtectedNetworkTargets = []string{
	// Localhost variants — anti SSRF balik ke kernel sendiri
	"127.0.0.1",
	"localhost",
	"::1",
	"0.0.0.0",

	// Cloud metadata IPs (AWS IMDS, GCP, Azure)
	"169.254.169.254", // AWS / GCP metadata
	"169.254.170.2",   // ECS task metadata
	"100.100.100.200", // Alibaba metadata
	"metadata.google.internal",
	"metadata.azure.com",

	// Link-local
	"169.254.",
	"fe80:",

	// Flowork kernel ports — anti AI hit kernel sendiri lewat HTTP loopback
	"http://localhost:3105",
	"http://localhost:3102",
	"http://localhost:3101",
	"http://127.0.0.1:3105",
	"http://127.0.0.1:3102",
	"http://127.0.0.1:3101",
}

// ─── 4. Privilege Escalation Patterns ───────────────────────────────────────

// Note: Go regexp pakai RE2 syntax — NO negative lookahead `(?!...)`. Untuk
// pattern dengan exception (mis. sudo --version OK), pakai allow-list di
// MatchPrivilegeEscalation logic.
var PrivilegeEscalationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsudo\s+\S+`),                              // sudo <anything> (filter --version di logic)
	regexp.MustCompile(`(?i)\bsu\s+root\b`),                             // su root
	regexp.MustCompile(`(?i)\bsu\s+-\s*$`),                              // su -
	regexp.MustCompile(`(?i)\brunas\s+/user:`),                          // runas /user:Administrator
	regexp.MustCompile(`(?i)\bdoas\s+`),                                 // doas (BSD sudo)
	regexp.MustCompile(`(?i)\bpkexec\s+`),                               // pkexec (PolicyKit)
	regexp.MustCompile(`(?i)\bgksudo\s+`),                               // gksudo
	regexp.MustCompile(`(?i)\bUAC\b.*bypass`),                           // UAC bypass attempt
	regexp.MustCompile(`(?i)\bsetuid\b.*\b0\b`),                         // setuid 0 (root)
}

// sudoAllowedFlags — flag yang aman untuk sudo (cuma version/help check).
var sudoAllowedFlags = []string{"--version", "--help", "-V", "-h"}

// isSudoSafeUsage — return true kalau "sudo" dipakai cuma untuk version/help.
// Aman karena ngga eksekusi command apa pun.
func isSudoSafeUsage(command string) bool {
	low := strings.ToLower(command)
	if !strings.Contains(low, "sudo ") && !strings.HasPrefix(low, "sudo\t") {
		return false
	}
	for _, flag := range sudoAllowedFlags {
		if strings.Contains(low, "sudo "+flag) {
			return true
		}
	}
	return false
}

// ─── Helper: Check Command String ────────────────────────────────────────────

// MatchDangerousSyscall — return non-empty matched pattern string kalau
// command match dangerous syscall, empty kalau aman.
func MatchDangerousSyscall(command string) string {
	for _, p := range DangerousSyscallPatterns {
		if p.MatchString(command) {
			return p.String()
		}
	}
	return ""
}

// MatchProtectedSystemPath — return matched path prefix kalau target write
// ke system protected dir.
func MatchProtectedSystemPath(target string) string {
	low := strings.ToLower(target)
	for _, prefix := range ProtectedSystemPaths {
		lowPrefix := strings.ToLower(prefix)
		if strings.HasPrefix(low, lowPrefix) || strings.Contains(low, lowPrefix) {
			return prefix
		}
	}
	return ""
}

// MatchProtectedNetworkTarget — return matched target kalau URL/host point
// ke loopback / metadata / kernel sendiri.
func MatchProtectedNetworkTarget(target string) string {
	low := strings.ToLower(target)
	for _, t := range ProtectedNetworkTargets {
		lowT := strings.ToLower(t)
		if strings.Contains(low, lowT) {
			return t
		}
	}
	return ""
}

// MatchPrivilegeEscalation — return matched pattern kalau command attempt
// privilege escalation. Allow-list bypass untuk safe sudo usage (--version).
func MatchPrivilegeEscalation(command string) string {
	// Safe sudo usage (version check) → bypass
	if isSudoSafeUsage(command) {
		return ""
	}
	for _, p := range PrivilegeEscalationPatterns {
		if p.MatchString(command) {
			return p.String()
		}
	}
	return ""
}
