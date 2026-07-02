// Package shellsec — classifier bahaya perintah shell (dipakai bashTool + host exec).
// AKAR fix: deny-list lama pakai substring `strings.Contains` naif → gampang di-bypass
// (`rm  -rf   /` spasi dobel, `rm --recursive --force /` beda ejaan flag, huruf besar).
// Di sini: normalisasi whitespace + deteksi STRUKTURAL (flag-order/ejaan independent).
// DESAIN: ZERO false-positive — cuma blokir yang JELAS destruktif (ga nyentuh path
// sistem = ga diblok). Perintah `rm -rf ./build` / `rm -rf *` (relatif) TETAP boleh.
// Security primitive → di-FREEZE (rule §5.2). 📄 Dok: lock/shell-security.md
package shellsec

import (
	"os"
	"regexp"
	"strings"
)

// strictOn — switch GUI FLOWORK_SHELL_STRICT (default ON). Escape-hatch: kalau classifier
// terstruktur ini kegedean (false-positive tak terduga bikin kerjaan owner ke-blok), set 0/off
// di GUI → classifier ini OFF, tapi deny-list substring lama di call-site TETEP jalan (ga
// ilang proteksi total). Default ON = proteksi penuh.
func strictOn() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_SHELL_STRICT")))
	return v != "0" && v != "false" && v != "off"
}

var (
	wsRe     = regexp.MustCompile(`\s+`)
	forkBomb = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	subSplit = regexp.MustCompile(`[;|&\n]+`)
)

// dangerNorm — substring dicek SETELAH whitespace di-collapse + lowercase → nangkep
// varian spasi-dobel/kapital yang lolos dari deny-list lama.
var dangerNorm = []string{
	"rm --no-preserve-root",
	"mkfs", "dd if=/dev/zero", "dd if=/dev/random",
	"of=/dev/sd", "of=/dev/nvme", "of=/dev/mmcblk",
	"> /dev/sda", "> /dev/nvme", "> /dev/mmcblk",
	"shutdown", "reboot", "halt", "poweroff", "init 0", "init 6",
	"/etc/shadow", "/etc/sudoers", ".ssh/id_rsa", "/dev/mem",
}

// Dangerous — true + alasan kalau perintah JELAS destruktif. Aman dipanggil bareng
// deny-list lama (additive). cmd = raw command string.
func Dangerous(cmd string) (bool, string) {
	if !strictOn() {
		return false, "" // escape-hatch OFF → serahkan ke deny-list substring lama (call-site)
	}
	norm := strings.ToLower(wsRe.ReplaceAllString(strings.TrimSpace(cmd), " "))
	if norm == "" {
		return false, ""
	}
	if forkBomb.MatchString(norm) {
		return true, "fork bomb"
	}
	for _, p := range dangerNorm {
		if strings.Contains(norm, p) {
			return true, p
		}
	}
	return rmDanger(norm)
}

// rmDanger — `rm` recursive+force yang nyasar ke PATH SISTEM (/, ~, /etc, dll),
// apa pun urutan/ejaan flag-nya. Path relatif (., *, ./build) TIDAK diblok.
func rmDanger(norm string) (bool, string) {
	for _, part := range subSplit.Split(norm, -1) {
		toks := strings.Fields(part)
		rmIdx := -1
		for i, t := range toks {
			if t == "rm" || strings.HasSuffix(t, "/rm") {
				rmIdx = i
				break
			}
		}
		if rmIdx < 0 {
			continue
		}
		recursive, force := false, false
		var targets []string
		for _, t := range toks[rmIdx+1:] {
			switch {
			case t == "--recursive" || t == "--recursive=true":
				recursive = true
			case t == "--force":
				force = true
			case t == "--no-preserve-root":
				recursive, force = true, true
			case strings.HasPrefix(t, "--"):
				// flag panjang lain → abaikan
			case strings.HasPrefix(t, "-") && len(t) > 1:
				if strings.ContainsAny(t, "rR") {
					recursive = true
				}
				if strings.ContainsRune(t, 'f') {
					force = true
				}
			default:
				targets = append(targets, t)
			}
		}
		if recursive && force {
			for _, tg := range targets {
				if systemPath(tg) {
					return true, "rm recursive+force ke path sistem: " + tg
				}
			}
		}
	}
	return false, ""
}

// systemPath — path yang KATASTROFIK kalau dihapus rekursif. Sengaja KONSERVATIF:
// cuma absolut-sistem + home. Relatif (., *, ./x) BUKAN → ga bikin false-positive.
func systemPath(p string) bool {
	p = strings.TrimSpace(p)
	switch p {
	case "/", "/*", "~", "~/", "~/*", "$home", "$home/*", "${home}", "${home}/*":
		return true
	}
	for _, d := range []string{"/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/var", "/home", "/boot", "/root", "/sys", "/opt", "/dev", "/proc"} {
		if p == d || p == d+"/" || p == d+"/*" {
			return true
		}
	}
	return false
}
