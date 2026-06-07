//go:build darwin

package guardian

import (
	"fmt"
	"os/exec"
	"strings"
)

// osSealer (macOS) — pakai chflags uchg (user-immutable flag). uchg bisa di-set user (ga wajib
// root buat uchg), tapi root tetap bisa cabut; buat proteksi penuh owner jalanin sebagai root.
func osSealer() Sealer { return chflagsSealer{} }

type chflagsSealer struct{}

func (chflagsSealer) Name() string { return "chflags uchg" }

func (chflagsSealer) Seal(p string) error   { return chflags("uchg", p) }
func (chflagsSealer) Unseal(p string) error { return chflags("nouchg", p) }

func (chflagsSealer) IsSealed(p string) (bool, error) {
	// `ls -ldO` nampilin flags di kolom setelah grup (mis. "uchg"). Cara portabel di macOS.
	out, err := exec.Command("ls", "-ldO", p).Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), "uchg"), nil
}

func chflags(flag, p string) error {
	out, err := exec.Command("chflags", flag, p).CombinedOutput()
	if err != nil {
		return fmt.Errorf("chflags %s %s: %v (%s)", flag, p, err, strings.TrimSpace(string(out)))
	}
	return nil
}
