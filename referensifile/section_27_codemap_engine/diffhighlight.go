package codeindex

// diffhighlight.go — git-based recent commits → file path set.
//
// Per Ayah 2026-05-06: adopt fitur "Understand-Anything" (Lum1104). Diff
// highlight = file yang di-touch oleh N commit terakhir dapat warna
// nyala di Code Map graph. Sebelumnya githook.go cuma log impact, ngga
// expose ke frontend.
//
// Approach: pure git command line via os/exec, ngga butuh library tambahan.
// Skip silently kalau git ngga available atau workspace bukan repo —
// return empty set, frontend graceful.

import (
	"os/exec"
	"strings"
)

// RecentlyTouchedSet return set of file paths yang muncul di N commit
// terakhir di workspace. Map[path]bool untuk fast lookup.
//
// Pakai `git log -n N --name-only --pretty=format:` — output cuma
// nama file. Filter empty + dedup via map.
//
// Best-effort: error → return empty map (frontend tetep render graph
// tanpa highlight overlay).
func RecentlyTouchedSet(workspace string, lastNCommits int) map[string]bool {
	out := map[string]bool{}
	if lastNCommits <= 0 {
		lastNCommits = 5
	}
	cmd := exec.Command("git", "log", "-n",
		intToStr(lastNCommits), "--name-only", "--pretty=format:")
	cmd.Dir = workspace
	data, err := cmd.Output()
	if err != nil {
		return out // git not available or not a repo — silent skip
	}
	for _, line := range strings.Split(string(data), "\n") {
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		// Normalize backslash → forward slash (Windows compat dengan
		// codemap_nodes.path yang stored forward slash).
		p = strings.ReplaceAll(p, "\\", "/")
		out[p] = true
	}
	return out
}

// intToStr — strconv.Itoa wrap untuk avoid extra import (ada di sini saja).
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
