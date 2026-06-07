// === LOCKED FILE ===
// Status: STABLE — live-proven (tested via realistic mock harness + live Telegram token 2026-06-07).
// Plug-and-play connector: to change behavior swap the folder; do not hand-edit without owner approval.
// Owner: Aola Sahidin (Mr.Dev). Locked: 2026-06-07.

// Package main is the Flowork Slack CHANNEL — a loket-native adapter.
//
// A channel is a DUMB PIPE (roadmap §F): it owns no brains. It watches Slack,
// hands each user message to a target agent over the loket bus (bus.request), and
// sends the agent's reply back to Slack. ALL the thinking — LLM, tools, routing,
// memory — lives in the agent, never here. Swap the agent (mr-flow-next today,
// anything tomorrow) and the channel never changes; swap the channel (telegram,
// discord, slack) and the agent never changes. That decoupling is the whole point.
//
// WHY POLLING, NOT SOCKET MODE: Slack's realtime transport is Socket Mode — a
// WEBSOCKET. wasip1 (the sandbox this runs in) has no websocket, only the host's
// outbound-HTTP primitive. So, exactly as the Connections design dictates
// (wasm + HTTP + polling), this polls the REST conversations.history API per
// configured channel. The cost is a few seconds of latency and one request per
// channel per interval — acceptable for a dumb pipe, and it keeps the connector a
// single portable wasm with no native deps.
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
	"hash/fnv"
	"os"
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
	return "slack-channel"
}

// hostFetch is the one outbound-HTTP primitive, used for BOTH the loket and the
// Slack API. Returns (status, body); (0, nil) on host error.
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
// over the loket bus and return the agent's reply text. It is Slack-agnostic, so
// handle_update can exercise the whole channel→bus→agent path with no live bot.
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

// chanSession turns a Slack channel id (alphanumeric, e.g. "C0123ABCD") into a
// stable positive int64 so each channel gets its own agent session key (the bus
// payload's chat_id). FNV-1a — cheap, deterministic, collision-safe enough here.
func chanSession(channelID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(channelID))
	return int64(h.Sum64() >> 1) // >>1 keeps it non-negative
}

// ── Slack I/O (only used live, when a token is set) ─────────────────────────

const slackAPI = "https://slack.com/api"

func authHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json; charset=utf-8",
	}
}

type slMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	User    string `json:"user"`
	BotID   string `json:"bot_id"`
	Text    string `json:"text"`
	TS      string `json:"ts"`
}

// getHistory fetches recent messages from a channel. With oldest != "" it returns
// only messages strictly newer (inclusive=false). Slack returns newest-first.
func getHistory(token, channelID, oldest string, limit int) ([]slMessage, error) {
	url := fmt.Sprintf("%s/conversations.history?channel=%s&limit=%d&inclusive=false", slackAPI, channelID, limit)
	if oldest != "" {
		url += "&oldest=" + oldest
	}
	st, raw := hostFetch("GET", url, authHeaders(token), nil)
	if st == 0 || st >= 400 {
		return nil, fmt.Errorf("slack conversations.history status=%d", st)
	}
	var env struct {
		OK       bool        `json:"ok"`
		Error    string      `json:"error"`
		Messages []slMessage `json:"messages"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return nil, fmt.Errorf("slack history decode")
	}
	if !env.OK {
		return nil, fmt.Errorf("slack history not ok: %s", env.Error)
	}
	return env.Messages, nil
}

// latestTS returns the ts of the most recent message in a channel (the cursor
// seed), so boot doesn't replay history. Empty string if the channel has none.
func latestTS(token, channelID string) string {
	msgs, err := getHistory(token, channelID, "", 1)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	return msgs[0].TS
}

func sendMessage(token, channelID, text string) error {
	if len(text) > 3900 {
		text = text[:3900] + "\n…(truncated)"
	}
	body, _ := json.Marshal(map[string]any{"channel": channelID, "text": text})
	st, raw := hostFetch("POST", slackAPI+"/chat.postMessage", authHeaders(token), body)
	if st == 0 || st >= 400 {
		return fmt.Errorf("slack chat.postMessage status=%d", st)
	}
	// Slack returns HTTP 200 even on logical failure; check the ok flag.
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &env) == nil && !env.OK {
		return fmt.Errorf("slack postMessage not ok: %s", env.Error)
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

const pollInterval = 3 * time.Second // REST poll cadence (no socket-mode here)

// boot is the live daemon: for each configured channel, poll for new (non-bot)
// messages → forward to the agent → send the reply back. IDLE (clean exit) when no
// token or no channels, so it can sit loaded beside other channels.
func boot() {
	token := strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN"))
	channels := parseChannels(os.Getenv("SLACK_CHANNELS"))
	if token == "" || len(channels) == 0 {
		fmt.Fprintf(os.Stderr, "[%s] no SLACK_BOT_TOKEN/SLACK_CHANNELS — channel IDLE (owner sets token + channel IDs to go live)\n", selfID())
		emit(map[string]any{"ok": true, "status": "idle (no token/channels)"})
		return
	}
	target := targetAgent()
	// Seed each channel's cursor at its newest message so we never replay history.
	cursor := map[string]string{}
	for _, ch := range channels {
		cursor[ch] = latestTS(token, ch)
	}
	fmt.Fprintf(os.Stderr, "[%s] live: target=%s channels=%d\n", selfID(), target, len(channels))
	for {
		for _, ch := range channels {
			msgs, err := getHistory(token, ch, cursor[ch], 50)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] getHistory(%s): %v\n", selfID(), ch, err)
				continue
			}
			if len(msgs) == 0 {
				continue
			}
			// Slack returns newest-first; advance the cursor to the newest ts seen
			// (index 0) BEFORE filtering, so skipped bot messages aren't refetched.
			cursor[ch] = msgs[0].TS
			// Process oldest-first.
			for i := len(msgs) - 1; i >= 0; i-- {
				m := msgs[i]
				// Skip bot messages (incl. ourselves) and non-plain events
				// (joins, edits…). A real user message has no subtype + no bot_id.
				if m.BotID != "" || m.Subtype != "" || strings.TrimSpace(m.Text) == "" {
					continue
				}
				reply := forwardToAgent(target, m.Text, chanSession(ch), m.User)
				if reply != "" {
					if err := sendMessage(token, ch, reply); err != nil {
						fmt.Fprintf(os.Stderr, "[%s] sendMessage: %v\n", selfID(), err)
					}
				}
			}
		}
		time.Sleep(pollInterval)
	}
}

// handleUpdate processes ONE message — the testable core, callable via RPC with a
// synthetic update so the channel→bus→agent path is verifiable with no live bot.
// Set "send":true (with a token + channel_id) to actually relay to Slack.
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
	reply := forwardToAgent(target, in.Text, chanSession(in.ChannelID), in.User)
	sent := false
	if in.Send {
		if token := strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN")); token != "" && in.ChannelID != "" {
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
