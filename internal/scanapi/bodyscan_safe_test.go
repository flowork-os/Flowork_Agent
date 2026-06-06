package scanapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeBodyScanRoot(t *testing.T) {
	deny := []string{"/", "/etc", "/etc/passwd", "/root", "/usr/bin", "/var/log", "/sys", "/proc/1"}
	for _, d := range deny {
		if safeBodyScanRoot(d) {
			t.Errorf("%q should be denied", d)
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" && safeBodyScanRoot(home) {
		t.Errorf("home root %q should be denied (too broad)", home)
	}
	// a project repo under home is allowed
	if home != "" {
		repo := filepath.Join(home, "Documents", "Flowork_Agent")
		if !safeBodyScanRoot(repo) {
			t.Errorf("%q (a repo) should be allowed", repo)
		}
	}
}
