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

//go:wasmimport flowork host_log_interaction
func hostLogInteraction(reqPtr, reqLen, outPtr, outMax uint32) uint32

//go:wasmimport flowork host_log_decision
func hostLogDecision(reqPtr, reqLen, outPtr, outMax uint32) uint32

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

// === Prompt Budget (per doc/standar_ai_agent.md section 11) ===
//
// Over-prompt = silent killer terutama buat local LLM (context window kecil).
// Setiap layer yang inject ke system prompt WAJIB respect budget di sini.
// On-demand fetch via tool call lebih baik daripada always-inject.
const (
	maxActiveSkills      = 3    // max skill auto-inject ke persona (sisanya via skill_search)
	maxSkillCharsPerItem = 300  // truncate instruction skill kalau terlalu panjang
	maxPersonaTotalChars = 4000 // hard cap persona total (~1000 token approx)
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
				// Section 3: log decision skip_task (drop chat unauthorized).
				logDecision("skip_task",
					"chat_id ngga ada di TELEGRAM_ALLOWED_CHATS — drop",
					"success",
					map[string]any{
						"chat_id":    chatID,
						"message_id": u.Message.MessageID,
					},
					0)
				continue
			}
			fmt.Fprintf(os.Stderr, "[mr-flow] received chat=%d text=%q\n", chatID, truncStr(u.Message.Text, 80))
			logInteraction("telegram", "in", strconv.FormatInt(chatID, 10), u.Message.Text, map[string]any{
				"message_id": u.Message.MessageID,
				"update_id":  u.UpdateID,
			})
			sendTyping(token, chatID)
			reply := callLLM(cfg, u.Message.Text)
			// Detect LLM failure via exact known error prefixes from callLLM
			// (sumber: callLLM returns: "router error:", "decode:", "llm:",
			// "(no choices)", or "" for empty). JANGAN pakai "(LLM " — itu
			// ngga pernah keluar dan bisa false-positive reply LLM yang
			// kebetulan mulai "(LLM..." (audit Section 3 finding).
			origReply := reply
			llmFailed := reply == "" ||
				strings.HasPrefix(reply, "router error:") ||
				strings.HasPrefix(reply, "decode:") ||
				strings.HasPrefix(reply, "llm:") ||
				reply == "(no choices)"
			if reply == "" {
				reply = "(LLM returned no text)"
			}
			fmt.Fprintf(os.Stderr, "[mr-flow] reply len=%d preview=%q\n", len(reply), truncStr(reply, 80))
			if len(reply) > 3900 {
				reply = reply[:3900] + "\n…(truncated)"
			}
			// Section 3: log decision model_choice (sukses) atau escalate (fail).
			// Log `reply_head` capture origReply (sebelum overwrite ke fallback
			// "(LLM returned no text)") supaya debug actionable.
			if llmFailed {
				logDecision("escalate",
					"LLM call gagal / kosong — reply fallback ke user. Cek router :2402 + provider quota.",
					"fail",
					map[string]any{
						"model":      cfg.Router.Model,
						"router":     cfg.Router.URL,
						"reply_head": truncStr(origReply, 120),
					},
					0)
			} else {
				logDecision("model_choice",
					"Dispatch ke router primary model (sukses).",
					"success",
					map[string]any{
						"model":      cfg.Router.Model,
						"chat_id":    chatID,
						"reply_len":  len(origReply),
						"reply_head": truncStr(origReply, 120),
					},
					0)
			}
			if err := sendMessage(token, chatID, reply); err != nil {
				fmt.Fprintf(os.Stderr, "[mr-flow] sendMessage err: %v\n", err)
			} else {
				logInteraction("telegram", "out", strconv.FormatInt(chatID, 10), reply, map[string]any{
					"model":            cfg.Router.Model,
					"reply_to_message": u.Message.MessageID,
				})
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
// === Prompt Budget enforcement ===
// Auto-inject MAX maxActiveSkills (= 3) skill ke persona. Sisanya available
// tapi warga butuh panggil tool `skill_search` untuk fetch. Mencegah
// over-prompt yang bikin LLM halu (terutama local LLM context kecil).
//
// Per skill: max maxSkillCharsPerItem chars (truncate instruction). Per
// total persona: max maxPersonaTotalChars (hard cap).
func callLLM(cfg agentConfig, userText string) string {
	persona := cfg.Prompt
	if len(cfg.Skills) > 0 {
		// Auto-inject hanya N skill pertama (asumsi ordered by importance).
		active := cfg.Skills
		if len(active) > maxActiveSkills {
			active = active[:maxActiveSkills]
		}
		var lines []string
		for _, s := range active {
			instr := s.Instructions
			if len(instr) > maxSkillCharsPerItem {
				instr = instr[:maxSkillCharsPerItem] + "…"
			}
			lines = append(lines, fmt.Sprintf("- %s (trigger=%q): %s", s.ID, s.Trigger, instr))
		}
		if extra := len(cfg.Skills) - len(active); extra > 0 {
			lines = append(lines, fmt.Sprintf("…+%d skill lain (panggil `skill_search` kalau perlu)", extra))
		}
		persona += "\n\nSkill aktif:\n" + strings.Join(lines, "\n")
	}
	// Hard cap persona total — last-ditch defense.
	if len(persona) > maxPersonaTotalChars {
		persona = persona[:maxPersonaTotalChars] + "\n…[truncated to respect prompt budget]"
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

// logInteraction — append row ke tabel `interactions` di state.db agent
// ini lewat host capability. Best-effort (silent on error supaya daemon
// loop ngga crash kalau DB lock sementara). channel = 'telegram',
// direction = 'in' | 'out'.
//
// ⚠️ JANGAN baca log ini dari WASM ke system prompt. Akses cuma via
// HTTP endpoint atau tool call (lihat standar_ai_agent.md section 11).
//
// logBuf 4KB: host cap error message ke 400 char, tapi sukses response
// `{"ok":true}` 12B saja. 4KB jaga margin kalau metadata error JSON
// bengkak (path full, stacktrace mini).
var logBuf [4096]byte

// logDecision — append row ke tabel `decisions` di state.db agent ini
// lewat host capability `host_log_decision`. Best-effort (silent on error
// supaya daemon loop ngga crash kalau DB lock).
//
// decisionType: 'model_choice' | 'skip_task' | 'escalate' | 'tool_pick' | dst
// outcome: 'success' | 'fail' | 'pending' (kosong → 'pending' di host)
//
// ⚠️ JANGAN baca log ini ke system prompt. Akses cuma via HTTP endpoint
// atau tool call (lihat standar_ai_agent.md section 11).
var decisionBuf [4096]byte

func logDecision(decisionType, rationale, outcome string, inputs map[string]any, refInteractionID int64) {
	req := map[string]any{
		"decision_type": decisionType,
		"rationale":     rationale,
	}
	if outcome != "" {
		req["outcome"] = outcome
	}
	if len(inputs) > 0 {
		req["inputs"] = inputs
	}
	if refInteractionID > 0 {
		req["ref_interaction_id"] = refInteractionID
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] decision marshal: %v\n", err)
		return
	}
	written := hostLogDecision(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(decisionBuf[:]), uint32(len(decisionBuf)),
	)
	if written == 0 {
		fmt.Fprintf(os.Stderr, "[mr-flow] host_log_decision returned 0 bytes\n")
		return
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(decisionBuf[:written], &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] decision decode: %v\n", err)
		return
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "[mr-flow] decision err: %s\n", resp.Error)
	}
}

func logInteraction(channel, direction, actor, content string, metadata map[string]any) {
	req := map[string]any{
		"channel":   channel,
		"direction": direction,
		"actor":     actor,
		"content":   content,
	}
	if len(metadata) > 0 {
		req["metadata"] = metadata
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] log marshal: %v\n", err)
		return
	}
	written := hostLogInteraction(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(logBuf[:]), uint32(len(logBuf)),
	)
	if written == 0 {
		fmt.Fprintf(os.Stderr, "[mr-flow] host_log_interaction returned 0 bytes\n")
		return
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(logBuf[:written], &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] log decode: %v\n", err)
		return
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "[mr-flow] log err: %s\n", resp.Error)
	}
}
