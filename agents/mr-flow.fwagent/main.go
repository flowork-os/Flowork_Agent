// mr-flow — Telegram AI agent untuk Flowork.
//
// === Layout standar (HARDCODED, lihat doc/standar_ai_agent.md) ===
//
//   /workspace/             ← workspace privat agent (mount kernel)
//   /workspace/state.db     ← SQLite per-agent
//   /shared/<id>/tools/     ← tools yang agent bikin sendiri
//   /shared/<id>/job/       ← output kerjaan
//   /shared/<id>/document/  ← markdown/notes/report
//   /shared/<id>/media/     ← audio/video/image
//   /shared/<id>/cache/     ← cache temporary
//   /shared/<id>/log/       ← log
//   /shared/_global/        ← bahan bareng lintas-agent
//
// === Sumber config (kernel inject) ===
//
//   FLOWORK_AGENT_CONFIG   — JSON utuh (prompt, router, schedule, skills)
//   TELEGRAM_BOT_TOKEN     — secrets.TELEGRAM_BOT_TOKEN dari popup
//   TELEGRAM_ALLOWED_CHATS — secrets.TELEGRAM_ALLOWED_CHATS dari popup
//   FLOWORK_AGENT_ID       — id agent (mr-flow)
//
// ABI (command pattern): kernel invoke binary dengan
//   os.Args[0] = "agent"
//   os.Args[1] = function name (boot | handle_message | send_message)
//   os.Args[2] = args JSON
// Output: JSON response ke stdout.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

// === Path konstanta (HARDCODED standar Flowork) ===
const (
	WorkspacePrivate = "/workspace"            // mount per-agent (eksklusif)
	WorkspaceDB      = "/workspace/state.db"   // SQLite per-agent
	WorkspaceShared  = "/shared"               // mount shared workspace (root project)
)

const (
	defaultRouter  = "http://127.0.0.1:2402/v1/chat/completions"
	defaultModel   = "claude-haiku-4-5"
	defaultPersona = "Lo Mr.Flow — AI Agent di Flowork microkernel buat Mr.Dev. " +
		"Reply natural Bahasa Indonesia santai (bro/lo/gw OK), concise, no markdown headers. " +
		"Kalau gak yakin, bilang gak yakin. Hindari halu."
	pollTimeout  = 25 // seconds
	respBufBytes = 524288
)

var outBuf [respBufBytes]byte

// ── Config dari kernel (sumber SQLite-backed) ──────────────────────────────

// Skill — entry dari config.skills[].
type Skill struct {
	ID           string `json:"id"`
	Trigger      string `json:"trigger"`
	Instructions string `json:"instructions"`
}

type agentConfig struct {
	Prompt string `json:"prompt"`
	Router struct {
		URL   string `json:"url"`
		Model string `json:"model"`
	} `json:"router"`
	Skills []Skill `json:"skills"`
}

// loadConfig parse FLOWORK_AGENT_CONFIG kalau ada, fallback ke default.
func loadConfig() agentConfig {
	cfg := agentConfig{}
	cfg.Router.URL = defaultRouter
	cfg.Router.Model = defaultModel
	cfg.Prompt = defaultPersona

	raw := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_CONFIG"))
	if raw == "" {
		return cfg
	}
	var parsed agentConfig
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] FLOWORK_AGENT_CONFIG parse error: %v\n", err)
		return cfg
	}
	if parsed.Prompt != "" {
		cfg.Prompt = parsed.Prompt
	}
	if parsed.Router.URL != "" {
		cfg.Router.URL = parsed.Router.URL
	}
	if parsed.Router.Model != "" {
		cfg.Router.Model = parsed.Router.Model
	}
	cfg.Skills = parsed.Skills
	return cfg
}

// ── Entry ──────────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		emit(map[string]string{"error": "missing function"})
		return
	}
	fn := os.Args[1]
	argsRaw := ""
	if len(os.Args) >= 3 {
		argsRaw = os.Args[2]
	}
	switch fn {
	case "boot":
		runDaemon()
	case "handle_message":
		doHandle(argsRaw)
	case "send_message":
		doSendAdmin(argsRaw)
	default:
		emit(map[string]string{"error": "unknown function: " + fn})
	}
}

// ── Daemon ─────────────────────────────────────────────────────────────────

func runDaemon() {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		fmt.Fprintln(os.Stderr, "[mr-flow] TELEGRAM_BOT_TOKEN belum di-set. Buka Setting → Credentials di popup, tambahin key 'TELEGRAM_BOT_TOKEN' = bot token dari @BotFather.")
		emit(map[string]string{"error": "TELEGRAM_BOT_TOKEN not set"})
		return
	}
	allowedRaw := strings.TrimSpace(os.Getenv("TELEGRAM_ALLOWED_CHATS"))
	if allowedRaw == "" {
		fmt.Fprintln(os.Stderr, "[mr-flow] TELEGRAM_ALLOWED_CHATS belum di-set. Buka Setting → Credentials, tambahin key 'TELEGRAM_ALLOWED_CHATS' = chat_id (pisah koma kalau lebih dari satu).")
		emit(map[string]string{"error": "TELEGRAM_ALLOWED_CHATS not set"})
		return
	}
	allowed := map[int64]bool{}
	for _, s := range strings.Split(allowedRaw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			allowed[n] = true
		}
	}
	if len(allowed) == 0 {
		emit(map[string]string{"error": "no valid chat_id in TELEGRAM_ALLOWED_CHATS"})
		return
	}

	cfg := loadConfig()
	fmt.Fprintf(os.Stderr, "[mr-flow] daemon ready: %d allowed chats, router=%s model=%s, skills=%d\n",
		len(allowed), cfg.Router.URL, cfg.Router.Model, len(cfg.Skills))

	var offset int64
	for {
		updates, err := getUpdates(token, offset, pollTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mr-flow] getUpdates err: %v\n", err)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			chatID := u.Message.Chat.ID
			if !allowed[chatID] {
				fmt.Fprintf(os.Stderr, "[mr-flow] drop unauthorized chat=%d\n", chatID)
				continue
			}
			fmt.Fprintf(os.Stderr, "[mr-flow] received chat=%d text=%q\n", chatID, truncStr(u.Message.Text, 80))
			sendTyping(token, chatID)
			reply := callLLM(cfg, u.Message.Text)
			if reply == "" {
				reply = "(LLM returned no text)"
			}
			fmt.Fprintf(os.Stderr, "[mr-flow] reply len=%d preview=%q\n", len(reply), truncStr(reply, 80))
			if len(reply) > 3900 {
				reply = reply[:3900] + "\n…(truncated)"
			}
			if err := sendMessage(token, chatID, reply); err != nil {
				fmt.Fprintf(os.Stderr, "[mr-flow] sendMessage err: %v\n", err)
			}
		}
	}
}

// ── Telegram primitives ────────────────────────────────────────────────────

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type Chat struct {
	ID int64 `json:"id"`
}

func getUpdates(token string, offset int64, timeoutSec int) ([]Update, error) {
	u := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?timeout=%d&allowed_updates=%%5B%%22message%%22%%5D",
		token, timeoutSec,
	)
	if offset > 0 {
		u += fmt.Sprintf("&offset=%d", offset)
	}
	resp, err := fetch("GET", u, nil, nil, (timeoutSec+5)*1000)
	if err != nil {
		return nil, err
	}
	if resp.Status >= 400 {
		return nil, fmt.Errorf("telegram %d: %s", resp.Status, truncStr(string(resp.Body), 160))
	}
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		return nil, fmt.Errorf("telegram envelope ok=false: %s", truncStr(string(resp.Body), 160))
	}
	var updates []Update
	_ = json.Unmarshal(env.Result, &updates)
	return updates, nil
}

func sendMessage(token string, chatID int64, text string) error {
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := fetch("POST", u, map[string]string{"Content-Type": "application/json"}, body, 15_000)
	if err != nil {
		return err
	}
	if resp.Status >= 400 {
		return fmt.Errorf("telegram %d: %s", resp.Status, truncStr(string(resp.Body), 160))
	}
	return nil
}

func sendTyping(token string, chatID int64) {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "action": "typing"})
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", token)
	_, _ = fetch("POST", u, map[string]string{"Content-Type": "application/json"}, body, 5_000)
}

// ── LLM router ─────────────────────────────────────────────────────────────

// callLLM kirim user text + persona + skills metadata ke router.
//
// Skills disisipkan ke system prompt sebagai daftar: kalau user triggernya
// match (mis. /sum), LLM tau prosedur yang harus dijalanin. Trigger ngga
// di-enforce hard — LLM bisa fallback ke jawaban natural.
func callLLM(cfg agentConfig, userText string) string {
	persona := cfg.Prompt
	if len(cfg.Skills) > 0 {
		var lines []string
		for _, s := range cfg.Skills {
			line := fmt.Sprintf("- %s (trigger=%q): %s", s.ID, s.Trigger, s.Instructions)
			lines = append(lines, line)
		}
		persona += "\n\nSkill yang aktif:\n" + strings.Join(lines, "\n")
	}

	body, _ := json.Marshal(map[string]any{
		"model": cfg.Router.Model,
		"messages": []map[string]string{
			{"role": "system", "content": persona},
			{"role": "user", "content": userText},
		},
	})

	resp, err := fetch("POST", cfg.Router.URL,
		map[string]string{"Content-Type": "application/json"}, body, 90_000)
	if err != nil {
		return "router error: " + err.Error()
	}
	if resp.Status >= 400 {
		return fmt.Sprintf("router %d: %s", resp.Status, truncStr(string(resp.Body), 200))
	}
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(resp.Body, &openaiResp); err != nil {
		return "decode: " + err.Error()
	}
	if openaiResp.Error != nil {
		errBytes, _ := json.Marshal(openaiResp.Error)
		return "llm: " + string(errBytes)
	}
	if len(openaiResp.Choices) == 0 {
		return "(no choices)"
	}
	return openaiResp.Choices[0].Message.Content
}

// ── Direct RPC handlers ────────────────────────────────────────────────────

func doHandle(argsRaw string) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(argsRaw), &in); err != nil {
		emit(map[string]any{"reply": "parse: " + err.Error()})
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		emit(map[string]any{"reply": "kosong bro, kirim pesan dulu"})
		return
	}
	emit(map[string]any{"reply": callLLM(loadConfig(), in.Text)})
}

func doSendAdmin(argsRaw string) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		emit(map[string]string{"error": "TELEGRAM_BOT_TOKEN not set"})
		return
	}
	var in struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal([]byte(argsRaw), &in); err != nil {
		emit(map[string]string{"error": "parse: " + err.Error()})
		return
	}
	if in.ChatID == 0 || in.Text == "" {
		emit(map[string]string{"error": "chat_id + text required"})
		return
	}
	if err := sendMessage(token, in.ChatID, in.Text); err != nil {
		emit(map[string]string{"error": err.Error()})
		return
	}
	emit(map[string]any{"ok": true})
}

// ── HTTP wrapper via host capability ───────────────────────────────────────

type httpResp struct {
	Status int
	Body   []byte
}

func fetch(method, url string, headers map[string]string, body []byte, timeoutMS int) (*httpResp, error) {
	req := map[string]any{
		"method":         method,
		"url":            url,
		"timeout_ms":     timeoutMS,
		"max_resp_bytes": 4 << 20,
	}
	if len(headers) > 0 {
		req["headers"] = headers
	}
	if len(body) > 0 {
		req["body_base64"] = base64.StdEncoding.EncodeToString(body)
	}
	reqJSON, _ := json.Marshal(req)

	written := hostNetFetch(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(outBuf[:]), uint32(len(outBuf)),
	)
	if written == 0 {
		return nil, fmt.Errorf("host_net_fetch returned 0 bytes")
	}
	var hostResp struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:written], &hostResp); err != nil {
		return nil, fmt.Errorf("decode host response: %w", err)
	}
	if hostResp.Error != "" {
		return nil, fmt.Errorf("host: %s", hostResp.Error)
	}
	bodyBytes, _ := base64.StdEncoding.DecodeString(hostResp.BodyB64)
	return &httpResp{Status: hostResp.Status, Body: bodyBytes}, nil
}

func emit(v any) {
	body, _ := json.Marshal(v)
	fmt.Println(string(body))
}

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
