//go:build windows

package guardian

import (
	"fmt"
	"os/exec"
	"strings"
)

// osSealer (Windows) — pakai icacls deny-write ACE buat Everyone (S-1-1-0). Bikin file ga bisa
// ditulis/dihapus oleh proses biasa. Buat efek penuh, jalanin arm sebagai Administrator.
// CATATAN: jalur Windows belum di-test di mesin asli (dibangun di Linux) — perlu verifikasi.
func osSealer() Sealer { return icaclsSealer{} }

type icaclsSealer struct{}

func (icaclsSealer) Name() string { return "icacls deny-write" }

func (icaclsSealer) Seal(p string) error {
	// deny Write Data + Append + Write EA/Attr + Delete buat Everyone.
	return icacls(p, "/deny", "*S-1-1-0:(WD,AD,WEA,WA,DE)")
}

func (icaclsSealer) Unseal(p string) error {
	return icacls(p, "/remove:d", "*S-1-1-0")
}

func (icaclsSealer) IsSealed(p string) (bool, error) {
	out, err := exec.Command("icacls", p).CombinedOutput()
	if err != nil {
		return false, err
	}
	s := string(out)
	return strings.Contains(s, "Everyone:(DENY)") || strings.Contains(s, "(DENY)"), nil
}

func icacls(path string, args ...string) error {
	all := append([]string{path}, args...)
	out, err := exec.Command("icacls", all...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls %s: %v (%s)", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}
