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
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

var outBuf [262144]byte

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
	r, err := loketCall("bus.request", map[string]any{
		"to":      target,
		"payload": map[string]any{"text": text, "chat_id": chatID, "user": user},
	})
	if err != nil {
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
	} `json:"message"`
}

func getUpdates(token string, offset int64, timeoutSec int) ([]tgUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=%d&allowed_updates=%%5B%%22message%%22%%5D", token, timeoutSec)
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

func sendMessage(token string, chatID int64, text string) error {
	if len(text) > 3900 {
		text = text[:3900] + "\n…(truncated)"
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text, "disable_web_page_preview": true})
	st, _ := hostFetch("POST", fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token), map[string]string{"Content-Type": "application/json"}, body)
	if st == 0 || st >= 400 {
		return fmt.Errorf("telegram sendMessage status=%d", st)
	}
	return nil
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
	var offset int64
	for {
		updates, err := getUpdates(token, offset, pollTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] getUpdates: %v\n", selfID(), err)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
				continue
			}
			chatID := u.Message.Chat.ID
			if len(allowed) > 0 && !allowed[chatID] {
				continue
			}
			reply := forwardToAgent(target, u.Message.Text, chatID, u.Message.From.Username)
			if reply != "" {
				if err := sendMessage(token, chatID, reply); err != nil {
					fmt.Fprintf(os.Stderr, "[%s] sendMessage: %v\n", selfID(), err)
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
