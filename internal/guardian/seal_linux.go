//go:build linux

package guardian

import (
	"fmt"
	"os/exec"
	"strings"
)

// osSealer (Linux) — pakai chattr +i (immutable bit ext/btrfs/xfs). Bahkan OWNER ga bisa nulis/
// hapus file +i tanpa root mencabut bit-nya. Butuh root (CAP_LINUX_IMMUTABLE) → arm via sudo.
func osSealer() Sealer { return chattrSealer{} }

type chattrSealer struct{}

func (chattrSealer) Name() string { return "chattr+i" }

func (chattrSealer) Seal(p string) error   { return chattr("+i", p) }
func (chattrSealer) Unseal(p string) error { return chattr("-i", p) }

func (chattrSealer) IsSealed(p string) (bool, error) {
	out, err := exec.Command("lsattr", "-d", "--", p).Output()
	if err != nil {
		return false, err
	}
	// format lsattr: "----i---------e---- /path". Field-0 = atribut.
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return false, nil
	}
	return strings.ContainsRune(fields[0], 'i'), nil
}

func chattr(flag, p string) error {
	out, err := exec.Command("chattr", flag, "--", p).CombinedOutput()
	if err != nil {
		return fmt.Errorf("chattr %s %s: %v (%s)", flag, p, err, strings.TrimSpace(string(out)))
	}
	return nil
}
