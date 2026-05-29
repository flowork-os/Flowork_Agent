package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/fsutil"
)

// runGit runs a git command in dir and returns combined output.
func runGit(dir string, args ...string) (string, error) {
	return runCmd(dir, "git", args...)
}

// runCmd runs an arbitrary command in dir.
func runCmd(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// copyToClipboard writes text to the system clipboard.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default: // Linux
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip/xsel/wl-copy)")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// formatTokenCount formats token counts as "1.2k" / "1.2M" for readability.
func formatTokenCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// contextBar renders a simple 20-char ASCII progress bar for context usage.
func contextBar(pct float64) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 5) // 20 segments × 5% each
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
	return fmt.Sprintf("[%s] %.1f%%", bar, pct)
}

// safeDiv returns a/b or 0 if b==0.
func safeDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	return a / b
}

func saveTranscript(entries []transcriptEntry, path string) error {
	var sb strings.Builder
	sb.WriteString("# FLOWORK Session Transcript\n\n")
	sb.WriteString("Saved: " + time.Now().Format(time.RFC3339) + "\n\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "## [%s] %s\n\n%s\n\n", e.Role, e.Title, e.Body)
	}
	return fsutil.SafeWriteFile(path, []byte(sb.String()), 0644)
}
