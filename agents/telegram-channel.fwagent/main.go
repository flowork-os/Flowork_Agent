// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// === LOCKED FILE ===
// Status: STABLE — live-proven (tested via realistic mock harness + live Telegram token 2026-06-07).
// Plug-and-play connector: to change behavior swap the folder; do not hand-edit without owner approval.
// Owner: Aola Sahidin (Mr.Dev). Locked: 2026-06-07.

// Package main is the Flowork Telegram CHANNEL — a loket-native adapter.
//
// A channel is a DUMB PIPE (roadmap §F): it owns no brains. It long-polls
// Telegram, hands each user message to a target agent over the loket bus
// (bus.request), and sends the agent's reply back to Telegram. ALL the thinking
// — LLM, tools, routing, memory — lives in the agent, never here. Swap the agent
// (mr-flow-next today, anything tomorrow) and the channel never changes; swap the
// channel (discord, cli, web) and the agent never changes. That decoupling is the
// whole point of a channel.
//
// Credentials (the bot token) are channel infrastructure, injected via the env as
// the agent secrets are. With NO token the boot daemon stays IDLE — so this can be
// built and loaded ALONGSIDE the legacy telegram daemon without both polling the
// same bot (which would steal each other's updates). The owner sets the token and
// swaps when ready.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

// 4 MiB response buffer — large enough to hold a base64-wrapped audio reply
// (TTS mp3) from the router; text/JSON responses use only a sliver of it.
var outBuf [4 << 20]byte

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func emit(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func selfID() string {
	if id := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_ID")); id != "" {
		return id
	}
	return "telegram-channel"
}

// hostFetch is the one outbound-HTTP primitive, used for BOTH the loket and the
// Telegram API. Returns (status, body); (0, nil) on host error.
func hostFetch(method, url string, headers map[string]string, body []byte) (int, []byte) {
	reqJSON, _ := json.Marshal(map[string]any{
		"method":         method,
		"url":            url,
		"timeout_ms":     65000,
		"max_resp_bytes": 4 << 20,
		"headers":        headers,
		"body_base64":    base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return 0, nil
	}
	var h struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(outBuf[:n], &h) != nil || h.Error != "" {
		return 0, nil
	}
	raw, _ := base64.StdEncoding.DecodeString(h.BodyB64)
	return h.Status, raw
}

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

// loketCall asks the kernel for a capability by name (the channel's only door to
// the agent, via bus.request).
func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})
	st, raw := hostFetch("POST", loketURL, map[string]string{"Content-Type": "application/json"}, body)
	if st == 0 {
		return nil, fmt.Errorf("loket: no response")
	}
	var res struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil, fmt.Errorf("loket decode")
	}
	if !res.OK {
		return nil, fmt.Errorf("loket refused: %s", res.Error)
	}
	return res.Result, nil
}

// targetAgent is which agent this channel feeds. Configurable; defaults to the
// loket-native Mr.Flow so a fresh copy works out of the box.
func targetAgent() string {
	if t := strings.TrimSpace(os.Getenv("TARGET_AGENT")); t != "" {
		return t
	}
	if raw := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_CONFIG")); raw != "" {
		var c struct {
			Target string `json:"target"`
			KV     struct {
				Target string `json:"target"`
			} `json:"kv"`
		}
		if json.Unmarshal([]byte(raw), &c) == nil {
			if c.Target != "" {
				return c.Target
			}
			if c.KV.Target != "" {
				return c.KV.Target
			}
		}
	}
	return "mr-flow-next"
}

// forwardToAgent is the channel's CORE: hand a user message to the target agent
// over the loket bus and return the agent's reply text. It is Telegram-agnostic,
// so handle_update can exercise the whole channel→bus→agent path with no live bot.
func forwardToAgent(target, text string, chatID int64, user string) string {
	if target == "" {
		target = targetAgent()
	}
	// Retry on transient failure: a blip (or an agent still warming right after a
	// restart) can make a single attempt fail even though the kernel is up. A few
	// short retries smooth that over; we don't wait forever, so a genuinely-down
	// agent still surfaces as an error instead of hanging the poll loop.
	var r json.RawMessage
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		r, err = loketCall("bus.request", map[string]any{
			"to":      target,
			"payload": map[string]any{"text": text, "chat_id": chatID, "user": user},
		})
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] bus.request failed after retries: %v\n", selfID(), err)
		return "[channel] agent error: " + err.Error()
	}
	// bus.request returns {"reply": <agent's raw emit>}; the agent's emit is itself
	// {"reply": "...", "agent": "..."}. Unwrap one level, then read the text.
	var outer struct {
		Reply json.RawMessage `json:"reply"`
	}
	if json.Unmarshal(r, &outer) == nil && len(outer.Reply) > 0 {
		var inner struct {
			Reply string `json:"reply"`
		}
		if json.Unmarshal(outer.Reply, &inner) == nil && inner.Reply != "" {
			return inner.Reply
		}
		// Some buses hand back the text directly as a JSON string.
		var s string
		if json.Unmarshal(outer.Reply, &s) == nil && s != "" {
			return s
		}
	}
	return "(agent ga balikin reply)"
}

// ── Voice: router STT/TTS + multipart (additive; degrades to text if no
// provider) ─────────────────────────────────────────────────────────────────
//
// A voice note is just another surface: download it, ask the ROUTER to
// transcribe (the router owns the STT provider — sovereign whisper or cloud, the
// channel doesn't care), forward the transcript to the agent exactly like text,
// then ask the router to speak the reply. If the router has no STT/TTS provider
// configured these return empty and the flow degrades cleanly to text.

func routerURL() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ROUTER_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:2402"
}

// tgBase is the Telegram API root. Configurable (TELEGRAM_API_BASE) so it can
// point at a self-hosted Bot API server or a test harness; defaults to the real
// cloud endpoint.
func tgBase() string {
	if v := strings.TrimSpace(os.Getenv("TELEGRAM_API_BASE")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.telegram.org"
}

// mpBoundary is a fixed multipart boundary — safe because our parts are short
// text fields + audio that won't contain this exact token.
const mpBoundary = "----floworkVoiceBoundaryX7MA4YWxkTrZu0gW"

// multipartBody assembles a multipart/form-data body (text fields + one file
// part) as raw bytes, so it works inside wasm with the single host_net_fetch
// primitive. Returns the Content-Type (with boundary) and the body bytes.
func multipartBody(fields map[string]string, fileField, fileName, fileMIME string, fileBytes []byte) (string, []byte) {
	var b []byte
	for k, v := range fields {
		b = append(b, ("--" + mpBoundary + "\r\n")...)
		b = append(b, ("Content-Disposition: form-data; name=\"" + k + "\"\r\n\r\n")...)
		b = append(b, v...)
		b = append(b, "\r\n"...)
	}
	if fileField != "" {
		b = append(b, ("--" + mpBoundary + "\r\n")...)
		b = append(b, ("Content-Disposition: form-data; name=\"" + fileField + "\"; filename=\"" + fileName + "\"\r\n")...)
		b = append(b, ("Content-Type: " + fileMIME + "\r\n\r\n")...)
		b = append(b, fileBytes...)
		b = append(b, "\r\n"...)
	}
	b = append(b, ("--" + mpBoundary + "--\r\n")...)
	return "multipart/form-data; boundary=" + mpBoundary, b
}

// routerTranscribe sends audio to the router STT and returns the transcript.
// Empty string on any failure (→ caller degrades to "no transcript").
func routerTranscribe(audio []byte, mime string) string {
	if mime == "" {
		mime = "audio/ogg"
	}
	ct, body := multipartBody(map[string]string{"model": "base"}, "file", "voice", mime, audio)
	st, raw := hostFetch("POST", routerURL()+"/v1/audio/transcriptions", map[string]string{"Content-Type": ct}, body)
	if st == 0 || st >= 400 {
		fmt.Fprintf(os.Stderr, "[%s] stt status=%d\n", selfID(), st)
		return ""
	}
	var res struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return ""
	}
	return strings.TrimSpace(res.Text)
}

// routerTTS asks the router to speak text and returns mp3 bytes (nil on failure).
func routerTTS(text string) []byte {
	voice := strings.TrimSpace(os.Getenv("TTS_VOICE"))
	if voice == "" {
		voice = "id-ID-ArdiNeural"
	}
	if len(text) > 1500 { // keep spoken replies concise (and the audio small)
		text = text[:1500]
	}
	body, _ := json.Marshal(map[string]any{"text": text, "voice": voice, "format": "mp3"})
	st, raw := hostFetch("POST", routerURL()+"/api/media-providers/tts", map[string]string{"Content-Type": "application/json"}, body)
	if st == 0 || st >= 400 {
		fmt.Fprintf(os.Stderr, "[%s] tts status=%d\n", selfID(), st)
		return nil
	}
	return raw
}

// ── Telegram I/O (only used live, when a token is set) ──────────────────────

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			Username string `json:"username"`
		} `json:"from"`
		Voice *struct {
			FileID   string `json:"file_id"`
			MimeType string `json:"mime_type"`
		} `json:"voice"`
	} `json:"message"`
}

// tgGetFilePath resolves a Telegram file_id to its downloadable file_path.
func tgGetFilePath(token, fileID string) string {
	st, raw := hostFetch("GET", fmt.Sprintf("%s/bot%s/getFile?file_id=%s", tgBase(), token, fileID), nil, nil)
	if st == 0 || st >= 400 {
		return ""
	}
	var res struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &res) != nil || !res.OK {
		return ""
	}
	return res.Result.FilePath
}

// tgDownloadFile fetches a Telegram file's raw bytes by its file_path.
func tgDownloadFile(token, filePath string) []byte {
	st, raw := hostFetch("GET", fmt.Sprintf("%s/file/bot%s/%s", tgBase(), token, filePath), nil, nil)
	if st == 0 || st >= 400 {
		return nil
	}
	return raw
}

// sendAudio uploads an mp3 reply to a chat (spoken reply for voice notes).
func sendAudio(token string, chatID int64, mp3 []byte) error {
	ct, body := multipartBody(map[string]string{"chat_id": strconv.FormatInt(chatID, 10)}, "audio", "reply.mp3", "audio/mpeg", mp3)
	st, _ := hostFetch("POST", fmt.Sprintf("%s/bot%s/sendAudio", tgBase(), token), map[string]string{"Content-Type": ct}, body)
	if st == 0 || st >= 400 {
		return fmt.Errorf("telegram sendAudio status=%d", st)
	}
	return nil
}

func getUpdates(token string, offset int64, timeoutSec int) ([]tgUpdate, error) {
	url := fmt.Sprintf("%s/bot%s/getUpdates?timeout=%d&allowed_updates=%%5B%%22message%%22%%5D", tgBase(), token, timeoutSec)
	if offset > 0 {
		url += fmt.Sprintf("&offset=%d", offset)
	}
	st, raw := hostFetch("GET", url, nil, nil)
	if st == 0 || st >= 400 {
		return nil, fmt.Errorf("telegram getUpdates status=%d", st)
	}
	var env struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if json.Unmarshal(raw, &env) != nil || !env.OK {
		return nil, fmt.Errorf("telegram envelope bad")
	}
	return env.Result, nil
}

// registerCommands auto-syncs the Telegram slash-command menu with the owner's
// groups: it asks the target agent (Mr.Flow) for the group→command list over the
// bus (the dumb pipe never needs to know the groups itself — isolation holds) and
// forwards that list to Telegram's setMyCommands. So adding a group makes its slash
// command appear automatically on the next boot.
// registerCommands tries ONCE to sync the Telegram slash menu with Mr.Flow's groups.
// Returns true on success. Called from the poll loop until it succeeds, so it never
// blocks boot and naturally retries once Mr.Flow is reachable.
func registerCommands(token, target string) bool {
	r, err := loketCall("bus.request", map[string]any{"to": target, "payload": map[string]any{"text": "/__groupcmds__"}})
	if err != nil {
		return false
	}
	var outer struct {
		Reply json.RawMessage `json:"reply"`
	}
	if json.Unmarshal(r, &outer) != nil || len(outer.Reply) == 0 {
		return false
	}
	var inner struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal(outer.Reply, &inner) != nil || !strings.HasPrefix(strings.TrimSpace(inner.Reply), "{") {
		return false
	}
	// inner.Reply is already a setMyCommands body: {"commands":[…]}. Dedup: only
	// push to Telegram when the list actually changed (the loop re-checks often so a
	// newly-created group's command appears within minutes, without spamming the API).
	if strings.TrimSpace(inner.Reply) == lastSlashBody {
		return true
	}
	st, _ := hostFetch("POST", fmt.Sprintf("%s/bot%s/setMyCommands", tgBase(), token), map[string]string{"Content-Type": "application/json"}, []byte(inner.Reply))
	fmt.Fprintf(os.Stderr, "[%s] setMyCommands status=%d\n", selfID(), st)
	if st >= 200 && st < 300 {
		lastSlashBody = strings.TrimSpace(inner.Reply)
		return true
	}
	return false
}

// lastSlashBody is the last command list pushed to Telegram (process-lifetime memo
// for the dedup above). Empty until the first successful sync.
var lastSlashBody string

func sendMessage(token string, chatID int64, text string) error {
	// Telegram caps a message at 4096 chars; a long answer must be SPLIT into
	// several messages (not truncated). Send each chunk in order.
	for _, chunk := range chunkText(text, 3900) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": chunk, "disable_web_page_preview": true})
		st, _ := hostFetch("POST", fmt.Sprintf("%s/bot%s/sendMessage", tgBase(), token), map[string]string{"Content-Type": "application/json"}, body)
		if st == 0 || st >= 400 {
			return fmt.Errorf("telegram sendMessage status=%d", st)
		}
	}
	return nil
}

// chunkText splits text into pieces of at most max bytes, breaking on a newline
// (then a space) near the limit so multi-message replies stay readable.
func chunkText(text string, max int) []string {
	if len(text) <= max {
		return []string{text}
	}
	var chunks []string
	for len(text) > max {
		cut := max
		if i := strings.LastIndex(text[:max], "\n"); i > max/2 {
			cut = i
		} else if i := strings.LastIndex(text[:max], " "); i > max/2 {
			cut = i
		}
		chunks = append(chunks, strings.TrimRight(text[:cut], " \n"))
		text = strings.TrimLeft(text[cut:], " \n")
	}
	if len(text) > 0 {
		chunks = append(chunks, text)
	}
	return chunks
}

func parseAllowed(s string) map[int64]bool {
	out := map[int64]bool{}
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
			out[n] = true
		}
	}
	return out
}

const pollTimeout = 50 // long-poll seconds

// waitLoketReady blocks until the kernel loket is reachable (or ~15s elapses).
// AutoBootDaemons can spawn this daemon before the kernel's HTTP surface (:1987)
// is fully up; without this gate a message already queued at startup would be
// processed too early and fail with "loket: no response". Pinging the always-safe
// time.now cap until it answers makes the daemon startup-race-proof.
func waitLoketReady() {
	for i := 0; i < 30; i++ {
		if _, err := loketCall("time.now", map[string]any{}); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// boot is the live daemon: long-poll Telegram → forward each allowed message to
// the agent → send the reply back. IDLE (clean exit) when no token, so it can sit
// loaded beside the legacy telegram daemon without both polling the same bot.
func boot() {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		fmt.Fprintf(os.Stderr, "[%s] no TELEGRAM_BOT_TOKEN — channel IDLE (owner sets token + swaps to go live)\n", selfID())
		emit(map[string]any{"ok": true, "status": "idle (no token)"})
		return
	}
	target := targetAgent()
	allowed := parseAllowed(os.Getenv("TELEGRAM_ALLOWED_CHATS"))
	fmt.Fprintf(os.Stderr, "[%s] live: target=%s allowed=%d\n", selfID(), target, len(allowed))
	waitLoketReady()
	var offset int64
	tick := 0
	for {
		// Re-sync the Telegram slash menu periodically (~every 6 polls) so a group
		// created/deleted at runtime auto-appears/disappears — plug-and-play, no
		// restart. registerCommands dedups, so it only calls setMyCommands on change.
		if tick%6 == 0 {
			registerCommands(token, target)
		}
		tick++
		updates, err := getUpdates(token, offset, pollTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] getUpdates: %v\n", selfID(), err)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil {
				continue
			}
			chatID := u.Message.Chat.ID
			if len(allowed) > 0 && !allowed[chatID] {
				continue
			}
			text := strings.TrimSpace(u.Message.Text)
			spoken := false
			// Voice note (additive): no text but a voice payload → download +
			// transcribe via the router, then treat the transcript as the text.
			if text == "" && u.Message.Voice != nil && u.Message.Voice.FileID != "" {
				if fp := tgGetFilePath(token, u.Message.Voice.FileID); fp != "" {
					if audio := tgDownloadFile(token, fp); len(audio) > 0 {
						text = routerTranscribe(audio, u.Message.Voice.MimeType)
						spoken = true
					}
				}
			}
			if text == "" {
				continue
			}
			reply := forwardToAgent(target, text, chatID, u.Message.From.Username)
			if reply == "" {
				continue
			}
			if err := sendMessage(token, chatID, reply); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] sendMessage: %v\n", selfID(), err)
			}
			// If the user spoke, speak the reply back too (best-effort; silently
			// skipped if the router has no TTS provider).
			if spoken {
				if mp3 := routerTTS(reply); len(mp3) > 0 {
					if err := sendAudio(token, chatID, mp3); err != nil {
						fmt.Fprintf(os.Stderr, "[%s] sendAudio: %v\n", selfID(), err)
					}
				}
			}
		}
	}
}

// handleUpdate processes ONE message — the testable core, callable via RPC with a
// synthetic update so the channel→bus→agent path is verifiable with no live bot.
// Set "send":true (with a token + chat_id) to actually relay to Telegram.
func handleUpdate(argsJSON string) {
	var in struct {
		Text   string `json:"text"`
		ChatID int64  `json:"chat_id"`
		User   string `json:"user"`
		Target string `json:"target"`
		Send   bool   `json:"send"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	target := in.Target
	if target == "" {
		target = targetAgent()
	}
	reply := forwardToAgent(target, in.Text, in.ChatID, in.User)
	sent := false
	if in.Send {
		if token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); token != "" && in.ChatID != 0 {
			sent = sendMessage(token, in.ChatID, reply) == nil
		}
	}
	emit(map[string]any{"reply": reply, "target": target, "sent": sent})
}

// handleVoice is the testable voice core: base64 audio → router STT → forward
// the transcript to the agent → (speak) router TTS. Callable via RPC so the whole
// voice pipe is verifiable with no live Telegram bot.
func handleVoice(argsJSON string) {
	var in struct {
		AudioB64 string `json:"audio_b64"`
		MIME     string `json:"mime"`
		Target   string `json:"target"`
		Speak    bool   `json:"speak"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	audio, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.AudioB64))
	if err != nil || len(audio) == 0 {
		emit(map[string]any{"error": "bad audio_b64"})
		return
	}
	target := in.Target
	if target == "" {
		target = targetAgent()
	}
	transcript := routerTranscribe(audio, in.MIME)
	if transcript == "" {
		emit(map[string]any{"error": "transcribe failed (no STT provider or empty audio)", "transcript": ""})
		return
	}
	reply := forwardToAgent(target, transcript, 0, "voice")
	out := map[string]any{"transcript": transcript, "reply": reply, "target": target}
	if in.Speak {
		if mp3 := routerTTS(reply); len(mp3) > 0 {
			out["audio_b64"] = base64.StdEncoding.EncodeToString(mp3)
			out["audio_bytes"] = len(mp3)
		}
	}
	emit(out)
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	fn := os.Args[1]
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch fn {
	case "boot":
		boot()
	case "handle_update":
		handleUpdate(args)
	case "handle_voice":
		handleVoice(args)
	case "handle":
		// Loket-bus invocation — unwrap the Message payload, treat as an update.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		p := args
		if len(msg.Payload) > 0 {
			p = string(msg.Payload)
		}
		handleUpdate(p)
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}
