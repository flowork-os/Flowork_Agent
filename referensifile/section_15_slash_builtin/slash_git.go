package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/fsutil"
)

// handleGitSlash menangani slash commands kategori git/PR/branch:
// diff/commit/commit-push-pr/branch/pr-comments/add-dir/files/copy.
// Returns true kalau cmd dikenali (handled), false kalau bukan kategori ini.
func (m *model) handleGitSlash(cmd string, args []string) bool {
	switch cmd {
	case "add-dir":
		if len(args) == 0 {
			dirs := append([]string{m.workspace}, m.additionalDirs...)
			m.appendLocal("ADD-DIR", fmt.Sprintf("Current dirs in scope:\n  %s\n\nUsage: /add-dir <path>", strings.Join(dirs, "\n  ")))
		} else {
			addPath := args[0]
			if !filepath.IsAbs(addPath) {
				addPath = filepath.Join(m.workspace, addPath)
			}
			if _, err := fsutil.SafeStat(addPath); err != nil {
				m.appendLocal("ADD-DIR", "Directory not found: "+addPath)
				return true
			}
			m.additionalDirs = append(m.additionalDirs, addPath)
			m.appendLocal("ADD-DIR", fmt.Sprintf("✅ Added %s to workspace scope.\nDirs: %s",
				addPath, strings.Join(append([]string{m.workspace}, m.additionalDirs...), ", ")))
		}

	case "files":
		searchPath := m.workspace
		if len(args) > 0 {
			searchPath = filepath.Join(m.workspace, strings.Join(args, " "))
		}
		dirEntries, err := os.ReadDir(searchPath)
		if err != nil {
			m.appendLocal("FILES", "Error reading directory: "+err.Error())
			return true
		}
		var sb strings.Builder
		dirs, files := 0, 0
		for _, e := range dirEntries {
			if e.IsDir() {
				sb.WriteString("  📁 " + e.Name() + "/\n")
				dirs++
			} else {
				sb.WriteString("  📄 " + e.Name() + "\n")
				files++
			}
		}
		m.appendLocal("FILES", fmt.Sprintf("%s  (%d dirs, %d files)\n\n%s", searchPath, dirs, files, sb.String()))

	case "copy":
		if len(m.entries) == 0 {
			m.appendLocal("COPY", "No messages to copy.")
			return true
		}
		// Find last assistant message
		text := ""
		for i := len(m.entries) - 1; i >= 0; i-- {
			if m.entries[i].Role == "assistant" || m.entries[i].Meta == "" {
				text = m.entries[i].Body
				break
			}
		}
		if text == "" {
			text = m.entries[len(m.entries)-1].Body
		}
		if err := copyToClipboard(text); err == nil {
			m.appendLocal("COPY", fmt.Sprintf("✅ Copied to clipboard (%d chars).", len(text)))
		} else {
			m.appendLocal("COPY", fmt.Sprintf("Clipboard unavailable: %s\n\nFirst 300 chars:\n%.300s", err, text))
		}

	case "diff":
		diffArgs := []string{"diff", "--color=never"}
		for _, a := range args {
			if a == "--staged" || a == "--cached" {
				diffArgs = append(diffArgs, "--staged")
			}
		}
		out, err := runGit(m.workspace, diffArgs...)
		if err != nil {
			m.appendLocal("DIFF", "git diff error: "+err.Error())
			return true
		}
		if strings.TrimSpace(out) == "" {
			out = "(no unstaged changes — try /diff --staged for staged changes)"
		}
		m.appendLocal("DIFF", out)

	case "commit":
		msg := ""
		if len(args) > 0 {
			msg = injectGitPrettyEmoji(strings.Join(args, " "))
		} else {
			msg = fmt.Sprintf("flowork: changes %s", time.Now().Format("2006-01-02 15:04"))
		}
		addOut, err := runGit(m.workspace, "add", "-A")
		if err != nil {
			m.appendLocal("COMMIT", "git add -A failed:\n"+addOut+"\n"+err.Error())
			return true
		}
		commitOut, err := runGit(m.workspace, "commit", "-m", msg)
		if err != nil {
			m.appendLocal("COMMIT", "git commit failed:\n"+commitOut+"\n"+err.Error())
			return true
		}
		m.appendLocal("COMMIT", "✅ "+commitOut)

	case "commit-push-pr":
		msg := "flowork: changes " + time.Now().Format("2006-01-02 15:04")
		if len(args) > 0 {
			msg = injectGitPrettyEmoji(strings.Join(args, " "))
		}
		if out, err := runGit(m.workspace, "add", "-A"); err != nil {
			m.appendLocal("COMMIT-PUSH-PR", "git add failed:\n"+out)
			return true
		}
		if out, err := runGit(m.workspace, "commit", "-m", msg); err != nil {
			m.appendLocal("COMMIT-PUSH-PR", "git commit failed:\n"+out)
			return true
		}
		if out, err := runGit(m.workspace, "push"); err != nil {
			m.appendLocal("COMMIT-PUSH-PR", "git push failed:\n"+out)
			return true
		}
		prOut, err := runCmd(m.workspace, "gh", "pr", "create", "--fill")
		if err != nil {
			m.appendLocal("COMMIT-PUSH-PR", "gh pr create failed:\n"+prOut+"\n"+err.Error())
			return true
		}
		m.appendLocal("COMMIT-PUSH-PR", "✅ Committed, pushed, and created PR:\n"+prOut)

	case "branch":
		if len(args) > 0 {
			out, err := runGit(m.workspace, "checkout", "-b", args[0])
			if err != nil {
				m.appendLocal("BRANCH", "git checkout -b failed:\n"+out+"\n"+err.Error())
			} else {
				m.appendLocal("BRANCH", "✅ "+strings.TrimSpace(out))
			}
		} else {
			current, _ := runGit(m.workspace, "rev-parse", "--abbrev-ref", "HEAD")
			list, _ := runGit(m.workspace, "branch", "--list")
			m.appendLocal("BRANCH", fmt.Sprintf("Current: %s\n\nAll branches:\n%s\n\nUsage: /branch <name>  → create & switch",
				strings.TrimSpace(current), list))
		}

	case "pr-comments":
		ghArgs := []string{"pr", "view", "--comments"}
		if len(args) > 0 {
			ghArgs = []string{"pr", "view", args[0], "--comments"}
		}
		out, err := runCmd(m.workspace, "gh", ghArgs...)
		if err != nil {
			m.appendLocal("PR-COMMENTS", "gh error (is gh installed and authenticated?):\n"+err.Error())
		} else {
			m.appendLocal("PR-COMMENTS", out)
		}

	case "upgrade":
		out, err := runGit(m.workspace, "pull")
		if err != nil {
			m.appendLocal("UPGRADE", "git pull failed: "+err.Error()+"\nManual: git pull && go build -o flowork ./cmd/flowork")
			return true
		}
		m.appendLocal("UPGRADE", "✅ Pulled latest:\n"+out+"\n\nRebuild: go build -o flowork ./cmd/flowork")

	default:
		return false
	}
	return true
}

// injectGitPrettyEmoji automatically prepends gitmoji for conventional commits.
func injectGitPrettyEmoji(msg string) string {
	lowerMsg := strings.ToLower(strings.TrimSpace(msg))

	prefixes := map[string]string{
		"feat":     "✨",
		"fix":      "🐛",
		"docs":     "📚",
		"style":    "💎",
		"refactor": "♻️",
		"perf":     "⚡️",
		"test":     "🚨",
		"build":    "📦",
		"ci":       "👷",
		"chore":    "🔧",
		"revert":   "⏪",
	}

	for prefix, emoji := range prefixes {
		if strings.HasPrefix(lowerMsg, prefix+":") || strings.HasPrefix(lowerMsg, prefix+"(") {
			if strings.HasPrefix(msg, emoji) {
				return msg
			}
			return emoji + " " + strings.TrimSpace(msg)
		}
	}
	return msg
}
