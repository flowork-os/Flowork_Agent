package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/agents"
	"github.com/teetah2402/flowork/internal/fsutil"
)

// handleMiscSlash menangani slash commands kategori chat / agent / IDE /
// browser / login / mesh / advisor / btw / feedback / lorem / stuck / stickers.
// Returns true kalau cmd dikenali (handled), false kalau bukan kategori ini.
func (m *model) handleMiscSlash(cmd string, args []string) bool {
	switch cmd {
	// ─── Phase 11: Swarm ───────────────────────────────────────────
	case "agents":
		m.appendLocal("AGENTS", agents.ListAgentTypesText())

	case "ide":
		port := os.Getenv("FLOW_BRIDGE_PORT")
		if port == "" {
			m.appendLocal("IDE", "Bridge tidak aktif.\nStart: flowork --bridge-port=5555\nLalu konek dari VS Code ext ke ws://localhost:5555")
		} else {
			m.appendLocal("IDE", fmt.Sprintf("✅ Bridge aktif di port %s → ws://localhost:%s", port, port))
		}
	case "chrome":
		url := "http://localhost:8899"
		if err := openURL(url); err != nil {
			m.appendLocal("CHROME", "Gagal buka browser: "+err.Error()+"\nManual: buka "+url)
		} else {
			m.appendLocal("CHROME", "Web chat dibuka → "+url)
		}
	case "desktop":
		url := "http://localhost:8899"
		if err := openChromeAppWindow(url); err != nil {
			m.appendLocal("DESKTOP", "Gagal buka chrome --app: "+err.Error()+"\nInstall Chrome/Edge, atau manual: chrome --app="+url)
		} else {
			m.appendLocal("DESKTOP", "Desktop window terbuka (chrome --app) → "+url)
		}
	case "mobile":
		ip := detectLANIP()
		if ip == "" {
			m.appendLocal("MOBILE", "Tidak bisa deteksi IP LAN. Cek `ipconfig` manual, lalu dari HP (WiFi sama) buka http://<IP>:8899.")
		} else {
			m.appendLocal("MOBILE", fmt.Sprintf("Buka dari HP (WiFi sama):\n  http://%s:8899\n\nFirewall Windows mungkin blokir — allow inbound port 8899 kalau perlu.", ip))
		}
	case "bridge":
		port := os.Getenv("FLOW_BRIDGE_PORT")
		if port == "" {
			m.appendLocal("BRIDGE", "Bridge server BELUM aktif.\nRestart dengan: flowork --bridge-port=5555")
		} else {
			m.appendLocal("BRIDGE", fmt.Sprintf("Bridge aktif di port %s → ws://localhost:%s", port, port))
		}
	case "login":
		var sb strings.Builder
		sb.WriteString("Auth status:\n\n")
		home, _ := os.UserHomeDir()
		if _, err := fsutil.SafeStat(filepath.Join(home, ".flowork", "owner.hash")); err == nil {
			sb.WriteString("  ✅ Owner password: configured\n")
		} else {
			sb.WriteString("  ⚠️  Owner password: belum diset (jalankan sekali dengan FLOWORK_OWNER_PASSWORD=...)\n")
		}
		for _, k := range []string{"ANTHROPIC_API_KEY", "DEEPSEEK_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "Nyawang_API_KEY", "GITHUB_TOKEN", "TELEGRAM_API_KEY"} {
			if os.Getenv(k) != "" {
				sb.WriteString("  ✅ " + k + "\n")
			} else {
				sb.WriteString("  ❌ " + k + "\n")
			}
		}
		m.appendLocal("LOGIN", strings.TrimRight(sb.String(), "\n"))
	case "logout":
		home, _ := os.UserHomeDir()
		hashPath := filepath.Join(home, ".flowork", "owner.hash")
		if err := fsutil.SafeRemove(hashPath); err == nil {
			m.appendLocal("LOGOUT", "✅ owner.hash dihapus. Session baru harus set ulang FLOWORK_OWNER_PASSWORD sekali.")
		} else if os.IsNotExist(err) {
			m.appendLocal("LOGOUT", "(owner.hash tidak ada — sudah logout)")
		} else {
			m.appendLocal("LOGOUT", "Gagal hapus: "+err.Error())
		}
	case "install-slack-app":
		m.appendLocal("SLACK", "Slack integration butuh OAuth + public callback endpoint. Roadmap v0.5.0.\nSementara: pakai flowork-telegram untuk bot chat cross-surface.")
	case "mesh":
		// Wrapper tipis ke binary flowork-mesh. /mesh status|start|join <pubkey>.
		// Jalankan synchronous untuk status/join; 'start' tetap sync tapi owner
		// sebaiknya pakai terminal terpisah agar daemon tidak di-block oleh TUI.
		sub := "status"
		subArgs := []string{}
		if len(args) > 0 {
			sub = args[0]
			subArgs = args[1:]
		}
		out, err := runFloworkMesh(m.workspace, append([]string{sub}, subArgs...)...)
		if err != nil {
			m.appendLocal("MESH", fmt.Sprintf("flowork-mesh %s gagal: %v\n%s", sub, err, out))
		} else {
			m.appendLocal("MESH", strings.TrimSpace(out))
		}
	case "advisor":
		m.appendLocal("ADVISOR", "Use /think to enable deep reasoning, then ask your question.\nOr prefix with 'think step by step:' in your prompt.")
	case "btw":
		if len(args) > 0 {
			note := strings.Join(args, " ")
			m.appendLocal("BTW", "📌 Noted: "+note)
		} else {
			m.appendLocal("BTW", "Usage: /btw <note> — adds a side note to the transcript")
		}
	case "feedback":
		m.appendLocal("FEEDBACK", "Report issues: github.com/teetah2402/flowork/issues\nEmail: bankakun2402@gmail.com")
	case "lorem":
		m.appendLocal("LOREM", "Ask AI: 'generate lorem ipsum placeholder text'")
	case "stuck":
		m.appendLocal("STUCK", "Try:\n  1. /think — enable deeper reasoning\n  2. /rewind — roll back last file changes\n  3. /compact --force — summarize context to free space\n  4. Ask AI: 'what are we stuck on?'")
	case "stickers":
		m.appendLocal("STICKERS", "🎉 FLOWORK Go v0.2.0 ✨\n🚀 Multi-provider AI CLI\n⚡ Selam · OpenAI · Kembar · Nyawang · Aksara\n🔧 42 tools · 18 skills · 78 slash commands")

	default:
		return false
	}
	return true
}

// handleSharedChatSlash menangani slash commands shared-chat.
// Returns true kalau cmd dikenali (handled), false kalau bukan kategori ini.
func (m *model) handleSharedChatSlash(cmd string, args []string) bool {
	switch cmd {
	// ── Shared-chat commands (same path as flowork-chat web UI) ────
	case "chat":
		// /chat <message> — post to the shared inbox on the active channel.
		// Omitting the message shows current channel and usage.
		ch := GetSharedChatChannel()
		if len(args) == 0 {
			m.appendLocal("CHAT", fmt.Sprintf(
				"Channel aktif: %s\n"+
					"Usage: /chat <pesan>  — kirim ke shared chat\n"+
					"       /channel <nama>  — ganti channel\n"+
					"       /private         — mulai sesi privat\n"+
					"       /clearchat       — clear channel aktif",
				ch))
		} else {
			msg := strings.Join(args, " ")
			postToSharedChat("user", msg)
			m.appendLocal("CHAT", fmt.Sprintf("[→ %s] %s", ch, msg))
		}

	case "channel":
		// /channel [name] — show or switch the shared-chat posting channel.
		if len(args) == 0 {
			m.appendLocal("CHANNEL", fmt.Sprintf(
				"Channel aktif: %s\nUsage: /channel <nama>", GetSharedChatChannel()))
		} else {
			ch := args[0]
			SetSharedChatChannel(ch)
			m.appendLocal("CHANNEL", fmt.Sprintf("✅ Channel berganti ke: %s", ch))
		}

	case "private":
		// /private [label] — buat personal channel yang unik, mulai sesi privat.
		label := fmt.Sprintf("%x", time.Now().UnixNano())[:8]
		if len(args) > 0 {
			label = args[0]
		}
		ch := "personal_" + label
		SetSharedChatChannel(ch)
		m.appendLocal("PRIVATE", fmt.Sprintf("✅ Sesi privat dimulai. Channel: %s\nGunakan /channel main untuk kembali ke grup.", ch))

	case "clearchat":
		// /clearchat — tulis sentinel __CLEAR__ ke channel aktif. flowork-chat
		// dan semua reader lain yang mendukung sentinel akan menyembunyikan
		// pesan sebelumnya. Tidak menghapus file, hanya menyembunyikan.
		ch := GetSharedChatChannel()
		clearSharedChatChannel(ch)
		m.appendLocal("CLEARCHAT", fmt.Sprintf("✅ Shared chat channel %q dibersihkan (sentinel ditulis).", ch))

	default:
		return false
	}
	return true
}
