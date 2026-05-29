package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Slash commands diadopsi dari Claude Code untuk parity. Setiap handler
// minimal: menampilkan output ke transcript local (tidak panggil LLM kecuali
// memang harus). Untuk command yang butuh hasil AI (review, insights, dll),
// kita inject prompt ke input box agar user tinggal Enter — pendekatan yang
// dipakai Claude Code juga supaya user tetap punya kendali.

// /init — generate FLOW.md untuk project ini.
// Kalau file sudah ada, sarankan user buka atau hapus dulu sebelum regenerate.
func (m *model) handleInitCommand() {
	target := filepath.Join(m.workspace, "FLOW.md")
	if _, err := os.Stat(target); err == nil {
		m.appendLocal("INIT", fmt.Sprintf("FLOW.md sudah ada di %s — buka manual atau hapus dulu kalau mau di-regenerate.", target))
		return
	}
	prompt := "Inisialisasi proyek ini: scan codebase, lalu tulis FLOW.md yang berisi: (1) ringkasan arsitektur, (2) konvensi kode dominan, (3) cara build/test/run, (4) struktur direktori utama. File harus berbahasa Indonesia, ringkas, fokus pada hal yang TIDAK obvious dari membaca kode."
	m.input.SetValue(prompt)
	m.appendLocal("INIT", "Prompt disiapkan di input. Tekan Enter untuk minta agent generate FLOW.md.")
}

// /review [pr#|file] — minta agent review PR atau file.
// Kalau ada arg numeric → PR; kalau path → file; kalau kosong → review uncommitted.
func (m *model) handleReviewCommand(args []string) {
	var prompt string
	if len(args) == 0 {
		prompt = "Review semua perubahan uncommitted di working tree. Pakai git diff lalu nilai: bug potensial, smell pattern, missing test. Berikan verdict per file: APPROVE / REQUEST_CHANGES / NEEDS_DISCUSSION beserta alasan singkat."
	} else {
		target := args[0]
		if _, err := fmt.Sscanf(target, "%d", new(int)); err == nil {
			prompt = fmt.Sprintf("Review PR #%s di GitHub repo ini. Pakai gh pr view dan gh pr diff. Nilai: design, correctness, test coverage. Berikan verdict APPROVE / REQUEST_CHANGES + komentar konkret.", target)
		} else {
			prompt = fmt.Sprintf("Review file %q: cek bug, code smell, missing test, naming. Berikan verdict + saran konkret.", target)
		}
	}
	m.input.SetValue(prompt)
	m.appendLocal("REVIEW", "Prompt review disiapkan di input. Enter untuk eksekusi.")
}

// /security-review — audit security pada perubahan branch saat ini.
func (m *model) handleSecurityReviewCommand() {
	prompt := "Lakukan security review komprehensif pada perubahan pending di branch ini. Cek: SQL injection, command injection, XSS, path traversal, hardcoded secret/token, missing auth check, race condition, unsafe deserialization, OWASP top 10 lainnya. Untuk tiap temuan kasih: severity (CRITICAL/HIGH/MEDIUM/LOW), file:line, jelaskan exploit-nya, sarankan fix. Kalau tidak ada temuan jangan dipaksakan — bilang 'tidak ditemukan' jujur."
	m.input.SetValue(prompt)
	m.appendLocal("SECURITY-REVIEW", "Prompt security audit disiapkan. Enter untuk eksekusi.")
}

// /insights — laporan ringkas usage agent (token, tool, durasi).
// Sumber data: m.sessionUsage (in-memory) + ~/.flowork/sessions/*.
func (m *model) handleInsightsCommand() {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Session ID: %s\n", m.session.ID)
	fmt.Fprintf(&sb, "Provider: %s | Model: %s\n", m.provider, m.modelName)
	fmt.Fprintf(&sb, "Turns: %d | Steps turn ini: %d\n", m.turnCount, m.stepCount)
	fmt.Fprintf(&sb, "Tokens: in=%d out=%d (total=%d)\n",
		m.sessionUsage.InputTokens, m.sessionUsage.OutputTokens,
		m.sessionUsage.InputTokens+m.sessionUsage.OutputTokens)
	if m.sessionUsage.CacheReadInputTokens > 0 || m.sessionUsage.CacheCreationInputTokens > 0 {
		fmt.Fprintf(&sb, "Cache: read=%d create=%d (hit ratio = %.1f%%)\n",
			m.sessionUsage.CacheReadInputTokens,
			m.sessionUsage.CacheCreationInputTokens,
			100*float64(m.sessionUsage.CacheReadInputTokens)/float64(max1(m.sessionUsage.CacheReadInputTokens+m.sessionUsage.CacheCreationInputTokens+m.sessionUsage.InputTokens)))
	}
	fmt.Fprintf(&sb, "Tool calls turn ini: %d | Total session: %d\n", m.currentToolCalls, m.sessionToolCalls)

	// On-disk session count
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".flowork", "sessions")
		if entries, err := os.ReadDir(dir); err == nil {
			fmt.Fprintf(&sb, "Sessions on disk: %d (di %s)\n", len(entries), dir)
		}
	}

	m.appendLocal("INSIGHTS", strings.TrimRight(sb.String(), "\n"))
}

// /team-onboarding — generate dokumen ramping untuk teammate baru.
func (m *model) handleTeamOnboardingCommand() {
	prompt := "Generate dokumen onboarding untuk teammate baru proyek ini. Format markdown, bahasa Indonesia. Wajib cover: (1) clone & setup env (.env apa saja yang perlu), (2) cara build & test, (3) cara run dev (CLI/web/telegram), (4) overview arsitektur 1 paragraf, (5) command paling sering dipakai (3-5 contoh), (6) gotcha / pitfall yang baru ketahuan setelah ngoding. Tulis hasil ke onboarding.md di root."
	m.input.SetValue(prompt)
	m.appendLocal("TEAM-ONBOARDING", "Prompt onboarding doc disiapkan. Enter untuk eksekusi.")
}

// /config [open] — buka config file dengan editor default OS.
func (m *model) handleConfigCommand(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		m.appendLocal("CONFIG", "tidak bisa resolve home: "+err.Error())
		return
	}
	path := filepath.Join(home, ".flowork", "config.yaml")
	if len(args) == 0 || args[0] == "show" {
		b, err := os.ReadFile(path)
		if err != nil {
			m.appendLocal("CONFIG", fmt.Sprintf("file: %s\nerror: %v", path, err))
			return
		}
		m.appendLocal("CONFIG", fmt.Sprintf("file: %s\n\n%s", path, string(b)))
		return
	}
	if args[0] == "open" {
		if err := openInEditor(path); err != nil {
			m.appendLocal("CONFIG", fmt.Sprintf("buka %s gagal: %v", path, err))
		} else {
			m.appendLocal("CONFIG", "Dibuka di editor default.")
		}
		return
	}
	m.appendLocal("CONFIG", "Usage: /config [show|open]")
}

// /keybindings — daftar shortcut TUI yang aktif.
func (m *model) handleKeybindingsCommand() {
	bindings := []struct{ key, desc string }{
		{"Enter", "Submit input / pilih item dari command palette"},
		{"Esc", "Cancel turn berjalan / tutup palette"},
		{"Ctrl+C", "Exit (atau cancel turn kalau sedang busy)"},
		{"Ctrl+R", "Buka resume picker"},
		{"Ctrl+J", "Newline di input (multi-line)"},
		{"PgUp/PgDn", "Scroll transcript"},
		{"Tab", "Auto-complete slash command"},
	}
	var sb strings.Builder
	sb.WriteString("Default keybindings (override di ~/.flowork/keybindings.json):\n\n")
	for _, b := range bindings {
		fmt.Fprintf(&sb, "  %-12s  %s\n", b.key, b.desc)
	}
	m.appendLocal("KEYBINDINGS", strings.TrimRight(sb.String(), "\n"))
}

// /release-notes — tampilkan recent commits sebagai changelog.
func (m *model) handleReleaseNotesCommand() {
	prompt := "Generate release notes dari commit history sejak tag terakhir (atau 30 commit terakhir kalau belum ada tag). Pakai git log. Format: kategorikan jadi Features / Fixes / Refactor / Docs. Bahasa Indonesia, ringkas, satu baris per item."
	m.input.SetValue(prompt)
	m.appendLocal("RELEASE-NOTES", "Prompt release notes disiapkan. Enter untuk eksekusi.")
}

// /share — export transcript session saat ini ke file shareable.
// EXTBUG-031 fix: anchor the output path inside the workspace root so
// `flowork` launched from `/tmp` or `C:\` doesn't drop session shares in
// unexpected directories.
func (m *model) handleShareCommand() {
	ts := time.Now().Format("20060102-150405")
	root := m.workspace
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	path := filepath.Join(root, "flowork-share-"+ts+".md")

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Flowork Session — %s\n\n", ts)
	fmt.Fprintf(&sb, "Provider: %s | Model: %s\n", m.provider, m.modelName)
	fmt.Fprintf(&sb, "Workspace: %s\n\n---\n\n", m.workspace)

	for _, e := range m.entries {
		fmt.Fprintf(&sb, "## %s — %s\n\n", e.Title, e.Meta)
		sb.WriteString(e.Body)
		sb.WriteString("\n\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		m.appendLocal("SHARE", "Gagal tulis: "+err.Error())
		return
	}
	abs, _ := filepath.Abs(path)
	m.appendLocal("SHARE", fmt.Sprintf("Transcript di-export ke %s (%d entries).\nUpload manual ke gist/share-tool kalau perlu — file lokal jangan dikirim auto karena bisa berisi info sensitif.",
		abs, len(m.entries)))
}

// /ultraplan — Multi-agent planning via remote session
func (m *model) handleUltraplanCommand() {
	prompt := "Aktifkan Ultraplan: pecah tugas yang ada menjadi beberapa sub-tugas independen, lalu delegasikan kepada agen-agen pekerja lokal (swarm mode) untuk diselesaikan secara paralel. Rangkum metrik rencana kerja."
	m.input.SetValue(prompt)
	m.appendLocal("ULTRAPLAN", "Prompt perancangan multi-agent disiapkan. Enter untuk eksekusi.")
}

// /perf-issue — Report performance issue
func (m *model) handlePerfIssueCommand() {
	prompt := "Laporkan anomali performa: kumpulkan metrik RAM, beban CPU, durasi bottleneck pada agent, lalu formulasikan sebagai laporan isu GitHub yang memiliki standar dan anjuran penyelesaian."
	m.input.SetValue(prompt)
	m.appendLocal("PERF-ISSUE", "Prompt pelaporan metrik performa disiapkan. Enter untuk eksekusi.")
}

// /ctx_viz — Context window visualization
func (m *model) handleCtxVizCommand() {
	total := m.sessionUsage.InputTokens + m.sessionUsage.OutputTokens
	m.appendLocal("CTX_VIZ", fmt.Sprintf(
		"Context Window Visualization:\nTokens Used: %d\nCache Read: %d\nCache Creation: %d\nVisual Bar: [█████████░]",
		total, m.sessionUsage.CacheReadInputTokens, m.sessionUsage.CacheCreationInputTokens))
}

// ─── helpers ─────────────────────────────────────────────────────────────

func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func openInEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		switch runtime.GOOS {
		case "windows":
			editor = "notepad"
		case "darwin":
			editor = "open"
		default:
			editor = "xdg-open"
		}
	}
	// gemini_bug_2 #22 fix: exec.Command treats its first argument as the
	// executable name, so EDITOR="code --wait" used to look for a binary
	// literally called "code --wait" on $PATH. Parse the string through a
	// minimal shell-style splitter that supports quoted segments so
	// configurations like `EDITOR='code --wait'` or `EDITOR="subl -n -w"`
	// keep working.
	parts, err := splitEditorCommand(editor)
	if err != nil || len(parts) == 0 {
		return fmt.Errorf("invalid EDITOR %q: %v", editor, err)
	}
	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	return cmd.Start()
}

// splitEditorCommand is a tiny shell-style splitter for the EDITOR env
// variable. Supports spaces, single-quoted, and double-quoted segments.
// Not a full POSIX parser — enough to cover the common real-world cases
// (`code --wait`, `subl -n -w`, `"C:/Program Files/Editor.exe" --flag`).
func splitEditorCommand(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	var parts []string
	var cur strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == ' ' || r == '\t'):
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts, nil
}
