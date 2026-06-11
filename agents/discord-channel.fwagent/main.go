// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// === LOCKED FILE ===
// Status: STABLE — live-proven (tested via realistic mock harness + live Telegram token 2026-06-07).
// Plug-and-play connector: to change behavior swap the folder; do not hand-edit without owner approval.
// Owner: Aola Sahidin (Mr.Dev). Locked: 2026-06-07.

// Package main is the Flowork Discord CHANNEL — a loket-native adapter.
//
// A channel is a DUMB PIPE (roadmap §F): it owns no brains. It watches Discord,
// hands each user message to a target agent over the loket bus (bus.request), and
// sends the agent's reply back to Discord. ALL the thinking — LLM, tools, routing,
// memory — lives in the agent, never here. Swap the agent (mr-flow-next today,
// anything tomorrow) and the channel never changes; swap the channel (telegram,
// discord, cli) and the agent never changes. That decoupling is the whole point.
//
// WHY POLLING, NOT THE GATEWAY: Discord delivers messages over a Gateway
// WEBSOCKET. wasip1 (the sandbox this runs in) has no websocket — only the host's
// outbound-HTTP primitive. So, exactly as the Connections design dictates
// (wasm + HTTP + polling), this polls the REST messages API
// (GET /channels/{id}/messages?after=) per configured channel. The cost is a few
// seconds of latency and one request per channel per interval — acceptable for a
// dumb pipe, and it keeps the connector a single portable wasm with no native deps.
//
// Credentials (the bot token) are channel infrastructure, injected via the env as
// the agent secrets are. With NO token the boot daemon stays IDLE — so this can be
// built and loaded ALONGSIDE other channels without polling anything. The owner
// sets the token + channel IDs to go live.
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
	return "discord-channel"
}

// hostFetch is the one outbound-HTTP primitive, used for BOTH the loket and the
// Discord API. Returns (status, body); (0, nil) on host error.
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
// over the loket bus and return the agent's reply text. It is Discord-agnostic,
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

// ── Discord I/O (only used live, when a token is set) ───────────────────────

const discordAPI = "https://discord.com/api/v10"

// authHeaders is the bot auth + content-type every Discord REST call needs.
func authHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bot " + token,
		"Content-Type":  "application/json",
	}
}

type dcAuthor struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

type dcMessage struct {
	ID        string   `json:"id"`
	Content   string   `json:"content"`
	ChannelID string   `json:"channel_id"`
	Author    dcAuthor `json:"author"`
}

// snowflake parses a Discord ID (a numeric snowflake string) for monotonic
// comparison. Snowflakes fit in uint64. Returns 0 on a malformed id.
func snowflake(id string) uint64 {
	n, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// getMessages fetches up to `limit` messages from a channel. With after != "" it
// returns only messages newer than that id. Discord returns newest-first.
func getMessages(token, channelID, after string, limit int) ([]dcMessage, error) {
	url := fmt.Sprintf("%s/channels/%s/messages?limit=%d", discordAPI, channelID, limit)
	if after != "" {
		url += "&after=" + after
	}
	st, raw := hostFetch("GET", url, authHeaders(token), nil)
	if st == 0 || st >= 400 {
		return nil, fmt.Errorf("discord getMessages status=%d", st)
	}
	var msgs []dcMessage
	if json.Unmarshal(raw, &msgs) != nil {
		return nil, fmt.Errorf("discord messages decode")
	}
	return msgs, nil
}

// latestID returns the id of the most recent message in a channel (the cursor
// seed), so boot doesn't replay history. Empty string if the channel has none.
func latestID(token, channelID string) string {
	msgs, err := getMessages(token, channelID, "", 1)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	return msgs[0].ID
}

func sendMessage(token, channelID, text string) error {
	if len(text) > 1900 { // Discord hard limit is 2000 chars
		text = text[:1900] + "\n…(truncated)"
	}
	body, _ := json.Marshal(map[string]any{"content": text})
	st, _ := hostFetch("POST", fmt.Sprintf("%s/channels/%s/messages", discordAPI, channelID), authHeaders(token), body)
	if st == 0 || st >= 400 {
		return fmt.Errorf("discord sendMessage status=%d", st)
	}
	return nil
}

func parseChannels(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}

const pollInterval = 3 * time.Second // REST poll cadence (no long-poll on Discord)

// boot is the live daemon: for each configured channel, poll for new (non-bot)
// messages → forward to the agent → send the reply back. IDLE (clean exit) when no
// token or no channels, so it can sit loaded beside other channels.
func boot() {
	token := strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN"))
	channels := parseChannels(os.Getenv("DISCORD_CHANNELS"))
	if token == "" || len(channels) == 0 {
		fmt.Fprintf(os.Stderr, "[%s] no DISCORD_BOT_TOKEN/DISCORD_CHANNELS — channel IDLE (owner sets token + channel IDs to go live)\n", selfID())
		emit(map[string]any{"ok": true, "status": "idle (no token/channels)"})
		return
	}
	target := targetAgent()
	// Seed each channel's cursor at its newest message so we never replay history.
	cursor := map[string]string{}
	for _, ch := range channels {
		cursor[ch] = latestID(token, ch)
	}
	fmt.Fprintf(os.Stderr, "[%s] live: target=%s channels=%d\n", selfID(), target, len(channels))
	for {
		for _, ch := range channels {
			msgs, err := getMessages(token, ch, cursor[ch], 50)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] getMessages(%s): %v\n", selfID(), ch, err)
				continue
			}
			// Discord returns newest-first; process oldest-first and advance the
			// cursor to the highest id seen.
			maxID := snowflake(cursor[ch])
			for i := len(msgs) - 1; i >= 0; i-- {
				m := msgs[i]
				if id := snowflake(m.ID); id > maxID {
					maxID = id
				}
				if m.Author.Bot || strings.TrimSpace(m.Content) == "" {
					continue // skip bots (incl. ourselves) and empty/embeds-only
				}
				chatID, _ := strconv.ParseInt(m.ChannelID, 10, 64)
				reply := forwardToAgent(target, m.Content, chatID, m.Author.Username)
				if reply != "" {
					if err := sendMessage(token, m.ChannelID, reply); err != nil {
						fmt.Fprintf(os.Stderr, "[%s] sendMessage: %v\n", selfID(), err)
					}
				}
			}
			if maxID > 0 {
				cursor[ch] = strconv.FormatUint(maxID, 10)
			}
		}
		time.Sleep(pollInterval)
	}
}

// handleUpdate processes ONE message — the testable core, callable via RPC with a
// synthetic update so the channel→bus→agent path is verifiable with no live bot.
// Set "send":true (with a token + channel_id) to actually relay to Discord.
func handleUpdate(argsJSON string) {
	var in struct {
		Text      string `json:"text"`
		ChannelID string `json:"channel_id"`
		User      string `json:"user"`
		Target    string `json:"target"`
		Send      bool   `json:"send"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	target := in.Target
	if target == "" {
		target = targetAgent()
	}
	chatID, _ := strconv.ParseInt(in.ChannelID, 10, 64)
	reply := forwardToAgent(target, in.Text, chatID, in.User)
	sent := false
	if in.Send {
		if token := strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")); token != "" && in.ChannelID != "" {
			sent = sendMessage(token, in.ChannelID, reply) == nil
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
