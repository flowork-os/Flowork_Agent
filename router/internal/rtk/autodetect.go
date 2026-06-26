// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package rtk

import (
	"regexp"
	"strings"
)

var (
	reGitDiffSig     = regexp.MustCompile(`(?m)^diff --git `)
	reGitDiffHunkSig = regexp.MustCompile(`(?m)^@@ `)
	reGitStatusSig   = regexp.MustCompile(`(?m)^On branch |^nothing to commit|^Changes (not |to be )|^Untracked files:`)
	rePorcelain      = regexp.MustCompile(`(?m)^[ MADRCU?!][ MADRCU?!] \S`)
	reBuildOutputSig = regexp.MustCompile(`(?im)^(npm (warn|error|ERR!)|yarn (warn|error)|\s*Compiling\s+\S+|\s*Downloading\s+\S+|added \d+ package|\[ERROR\]|BUILD (SUCCESS|FAILED)|\s*Finished\s+|Successfully (installed|built)|ERROR:)`)
	reTreeGlyph      = regexp.MustCompile("[├└]──|│  ")
	reLsRow          = regexp.MustCompile(`(?m)^[-dlbcps][rwx-]{9}`)
	reLsTotal        = regexp.MustCompile(`(?m)^total \d+$`)
	reSearchListHdr  = regexp.MustCompile(`(?m)^Files: `)
	reReadNumbered   = regexp.MustCompile(`(?m)^\s*\d+\|`)
)

func autoDetect(head string) Filter {

	filtersMu.RLock()
	by := make(map[string]Filter, len(filters))
	for _, f := range filters {
		by[f.Name()] = f
	}
	filtersMu.RUnlock()

	if reGitDiffSig.MatchString(head) || reGitDiffHunkSig.MatchString(head) {
		if f := by["git-diff"]; f != nil {
			return f
		}
	}

	if reGitStatusSig.MatchString(head) {
		if f := by["git-status"]; f != nil {
			return f
		}
	}

	if reBuildOutputSig.MatchString(head) {
		if f := by["build-output"]; f != nil {
			return f
		}
	}

	if isMostlyPorcelain(head) {
		if f := by["git-status"]; f != nil {
			return f
		}
	}

	lines := strings.Split(head, "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			nonEmpty = append(nonEmpty, ln)
		}
	}
	first5 := nonEmpty
	if len(first5) > 5 {
		first5 = first5[:5]
	}
	for _, ln := range first5 {
		if isGrepLine(ln) {
			if f := by["grep"]; f != nil {
				return f
			}
			break
		}
	}

	if len(nonEmpty) >= 3 {
		allPath := true
		for _, ln := range nonEmpty {
			if !isPathLike(ln) {
				allPath = false
				break
			}
		}
		if allPath {
			if f := by["find"]; f != nil {
				return f
			}
		}
	}

	if reTreeGlyph.MatchString(head) {
		if f := by["tree"]; f != nil {
			return f
		}
	}

	if reLsTotal.MatchString(head) || countRegexpMatches(reLsRow, head) >= 3 {
		if f := by["ls"]; f != nil {
			return f
		}
	}

	if reSearchListHdr.MatchString(head) {
		if f := by["search-list"]; f != nil {
			return f
		}
	}

	if countRegexpMatches(reReadNumbered, head) >= 10 {
		if f := by["read-numbered"]; f != nil {
			return f
		}
	}

	if len(nonEmpty) >= 5 {
		if f := by["dedup-log"]; f != nil {
			return f
		}
	}

	if strings.Count(head, "\n") >= 40 {
		if f := by["smart-truncate"]; f != nil {
			return f
		}
	}
	return nil
}

func isGrepLine(line string) bool {
	first := strings.IndexByte(line, ':')
	if first < 0 {
		return false
	}
	second := strings.IndexByte(line[first+1:], ':')
	if second < 0 {
		return false
	}
	num := line[first+1 : first+1+second]
	if num == "" {
		return false
	}
	for _, c := range num {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isPathLike(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if strings.ContainsRune(t, ':') {
		return false
	}
	return strings.HasPrefix(t, ".") || strings.HasPrefix(t, "/") || strings.ContainsRune(t, '/')
}

func isMostlyPorcelain(head string) bool {
	lines := strings.Split(head, "\n")
	nonEmpty := 0
	matches := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		nonEmpty++
		if rePorcelain.MatchString(ln) {
			matches++
		}
	}
	if nonEmpty < 3 {
		return false
	}
	return matches*100/nonEmpty >= 60
}

func countRegexpMatches(re *regexp.Regexp, s string) int {
	return len(re.FindAllString(s, -1))
}
