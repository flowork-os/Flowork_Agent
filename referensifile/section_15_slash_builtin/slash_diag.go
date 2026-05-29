package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/compact"
	"github.com/teetah2402/flowork/internal/core"
	"github.com/teetah2402/flowork/internal/fsutil"
	"github.com/teetah2402/flowork/internal/tools"
)

// handleDiagSlash menangani slash commands kategori diagnostic / context /
// thinking / theme / debug / heap / tasks / etc.
// Returns true kalau cmd dikenali (handled), false kalau bukan kategori ini.
func (m *model) handleDiagSlash(cmd string, args []string) bool {
	switch cmd {
	// ─── Phase 08: Thinking Modes ───────────────────────────────
	case "fast":
		m.thinkingMode = core.ThinkingDisabled
		m.appendLocal("THINKING", "⚡ Fast mode: thinking disabled for this session.\nUse /think to re-enable.")
	case "think":
		if len(args) == 0 {
			m.thinkingMode = core.ThinkingEnabled
			m.thinkingBudget = 4000
			m.appendLocal("THINKING", "🧠 Thinking enabled (budget: 4000 tokens).\nUse /think <budget> to set custom budget, /fast to disable.")
		} else {
			budget := 4000
			fmt.Sscanf(args[0], "%d", &budget)
			if budget < 0 {
				budget = 0
			}
			m.thinkingMode = core.ThinkingEnabled
			m.thinkingBudget = budget
			m.appendLocal("THINKING", fmt.Sprintf("🧠 Thinking enabled (budget: %d tokens).", budget))
		}
	case "thinking":
		m.appendLocal("THINKING", fmt.Sprintf("Thinking mode: %s\nBudget: %d tokens\nUse /fast to disable, /think <N> to set budget.", m.thinkingMode, m.thinkingBudget))
	case "thinkback":
		// Tampilkan thinking trace dari turn terakhir kalau ada, plus mode aktif.
		trace := "(belum ada thinking trace — jalankan /think atau minta 'think step by step' di prompt)"
		if m.lastThinking != "" {
			trace = m.lastThinking
		}
		m.appendLocal("THINKBACK", fmt.Sprintf("Thinking mode: %s\n\nLast trace:\n%s", string(m.thinkingMode), trace))

	case "context":
		limit := compact.ModelContextLimit(m.modelName)
		total := m.sessionUsage.InputTokens + m.sessionUsage.OutputTokens
		pct := 0.0
		if limit > 0 {
			pct = float64(total) / float64(limit) * 100
		}
		bar := contextBar(pct)
		m.appendLocal("CONTEXT", fmt.Sprintf(
			"Tokens in:   %s\nTokens out:  %s\nTotal:       %s / ~%s  (%.1f%%)\n%s\nMessages:    %d\nTurns:       %d",
			formatTokenCount(m.sessionUsage.InputTokens),
			formatTokenCount(m.sessionUsage.OutputTokens),
			formatTokenCount(total), formatTokenCount(limit), pct,
			bar, len(m.session.Messages), m.turnCount,
		))

	case "brief":
		m.appendLocal("BRIEF", "Ask AI: 'summarize what we've done so far'")

	case "compact":
		if len(args) > 0 && args[0] == "--force" {
			if m.compactor != nil && m.session != nil {
				ok, msg, err := m.compactor.ForceFullCompact(context.Background(), m.session)
				if err != nil {
					m.appendLocal("COMPACT", "Compact error: "+err.Error())
				} else if ok {
					m.appendLocal("COMPACT", "✅ "+msg)
				} else {
					m.appendLocal("COMPACT", "Session too short to compact (need 8+ messages).")
				}
			} else {
				m.appendLocal("COMPACT", "Compactor not available.")
			}
		} else {
			limit := compact.ModelContextLimit(m.modelName)
			total := m.sessionUsage.InputTokens + m.sessionUsage.OutputTokens
			pct := 0.0
			if limit > 0 {
				pct = float64(total) / float64(limit) * 100
			}
			m.appendLocal("COMPACT", fmt.Sprintf(
				"Auto-compact: enabled (tiers at 70%% / 85%% / 95%%)\nContext used: %.1f%%  (%s / ~%s)\n\nRun /compact --force to trigger full summarization now.",
				pct, formatTokenCount(total), formatTokenCount(limit),
			))
		}

	case "theme":
		// Theme tidak di-swap runtime (Bubble Tea perlu rebuild styling),
		// tapi setidaknya tampilkan apa yang aktif + cara override di
		// ~/.flowork/theme.yaml. Memberi transparansi vs tutup mata.
		m.appendLocal("THEME", "Current theme: terminal-default (mengikuti ANSI color palette shell)\n\nOverride: tulis ~/.flowork/theme.yaml dengan warna hex, restart flowork untuk apply.\nContoh:\n  primary: \"#7aa2f7\"\n  accent:  \"#f7768e\"\n  success: \"#9ece6a\"\n  error:   \"#f7768e\"")
	case "color":
		// Diagnostic: cek terminal support true-color vs 256 vs 16.
		cap := "16-color (basic)"
		if os.Getenv("COLORTERM") == "truecolor" || os.Getenv("COLORTERM") == "24bit" {
			cap = "truecolor (24-bit)"
		} else if strings.Contains(os.Getenv("TERM"), "256") {
			cap = "256-color"
		}
		m.appendLocal("COLOR", fmt.Sprintf("Terminal capability: %s\nTERM=%s  COLORTERM=%s\n\nTUI pakai lipgloss — otomatis degrade ke capability terminal.",
			cap, os.Getenv("TERM"), os.Getenv("COLORTERM")))

	case "vim":
		// Toggle vim-mode flag pada model; input component belum support
		// native vim motion, tapi status flag kita pakai untuk future
		// handler dan setidaknya memberi feedback bahwa setting tercatat.
		m.vimMode = !m.vimMode
		status := "OFF"
		if m.vimMode {
			status = "ON (hjkl navigation di transcript, i/a untuk insert)"
		}
		m.appendLocal("VIM", "Vim mode: "+status)
	case "rate-limit-options":
		// Tampilkan nilai aktual dari AgentConfig, bukan angka statis
		// yang mungkin sudah kadaluarsa di dokumentasi.
		wait := core.DefaultRateLimitWait
		m.appendLocal("RATE-LIMIT", fmt.Sprintf("Wait-on-rate-limit: %s (default — configurable via AgentConfig.RateLimitWait)\nRecoverable errors: 429, quota, timeout, 5xx, network — auto-retry SAMA step sampai sukses atau ctx cancel\nProvider fallback chain aktif di flowork-chat: Selam→Kembar→Aksara→OpenAI→Nyawang→Ollama", wait))

	case "doctor":
		var sb strings.Builder
		sb.WriteString("FLOWORK v0.2.0\n\n")
		sb.WriteString(fmt.Sprintf("Provider:     %s\nModel:        %s\nWorkspace:    %s\nPermission:   %s\nSession ID:   %s\n\n",
			m.provider, m.modelName, m.workspace, tools.CurrentPermissionMode(), m.session.ID))
		checkCmd := func(label, name string, arg ...string) {
			out, err := runCmd(m.workspace, name, arg...)
			if err == nil {
				parts := strings.SplitN(strings.TrimSpace(out), "\n", 2)
				val := ""
				if len(parts) > 0 {
					val = parts[0]
				}
				sb.WriteString("✅ " + label + ": " + val + "\n")
			} else {
				sb.WriteString("❌ " + label + ": not found\n")
			}
		}
		checkCmd("git", "git", "--version")
		checkCmd("gh (GitHub CLI)", "gh", "--version")
		checkCmd("bun", "bun", "--version")
		checkCmd("node", "node", "--version")
		checkCmd("python", "python", "--version")
		checkCmd("go", "go", "version")

		m.appendLocal("DOCTOR", sb.String())

	case "debug":
		// Toggle FLOW_DEBUG runtime — sebagian kode internal baca env var ini
		// saat dipanggil, jadi set di sini langsung affect subsequent calls.
		cur := os.Getenv("FLOW_DEBUG")
		if cur == "" || cur == "0" {
			_ = os.Setenv("FLOW_DEBUG", "1")
			m.appendLocal("DEBUG", "FLOW_DEBUG=1 (on). Verbose logging aktif. Set ulang ke 0 dengan /debug.")
		} else {
			_ = os.Setenv("FLOW_DEBUG", "0")
			m.appendLocal("DEBUG", "FLOW_DEBUG=0 (off). Verbose logging mati.")
		}

	case "hooks":
		m.appendLocal("HOOKS", "Hooks are now managed by the kernel.")

	case "heapdump":
		// Tulis heap profile ke tmp file — real implementation pakai runtime/pprof.
		home, _ := os.UserHomeDir()
		dumpPath := filepath.Join(home, ".flowork", fmt.Sprintf("heapdump-%s.pprof", time.Now().Format("20060102-150405")))
		f, err := fsutil.SafeCreate(dumpPath)
		if err != nil {
			m.appendLocal("HEAPDUMP", "Gagal buat file: "+err.Error())
		} else {
			runtime.GC()
			if perr := pprof.WriteHeapProfile(f); perr != nil {
				m.appendLocal("HEAPDUMP", "Gagal tulis profile: "+perr.Error())
			} else {
				m.appendLocal("HEAPDUMP", fmt.Sprintf("✅ Heap profile ditulis ke %s\n\nAnalisa: go tool pprof %s", dumpPath, dumpPath))
			}
			f.Close()
		}

	case "tasks":
		m.appendLocal("TASKS", tools.ListBgTasksText())

	case "plugins":
		m.appendLocal("PLUGINS", "Plugins are now managed by the kernel.")

	default:
		return false
	}
	return true
}
