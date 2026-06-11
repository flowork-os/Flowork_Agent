// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// Package main is the Flowork CONNECTOR TEMPLATE — copy this folder, fill in the
// three TODO(connector) spots, and you have a new plug-and-play connector for the
// Connections family (the same shape that drives Telegram today).
//
// ── What a connector IS ──────────────────────────────────────────────────────
// A DUMB PIPE. It forwards a line of text from an external surface (Telegram /
// Discord / Slack / email-API / …) to a target agent over the loket bus, and sends
// the agent's reply back. It owns NO brains — all thinking (LLM, tools, memory)
// lives in the agent. Swap the agent and the connector is unchanged; swap the
// connector and the agent is unchanged. That decoupling is the whole point.
//
// ── Why it is built this way (see ROADMAP_CONNECTIONS.md) ────────────────────
//   - WASM (wazero), pure-Go, built GOOS=wasip1 → ONE binary runs on Win/Mac/Linux.
//   - HTTP only, via the host_net_fetch import → use the platform's HTTP API; never
//     a raw socket (that would break portability and need a native build per OS).
//   - POLLING by default → works behind NAT on any desktop OS.
//   - Self-contained: this folder is everything. Crash here touches only this folder.
//
// ── How to make your own ─────────────────────────────────────────────────────
//  1. Copy this folder to agents/<your-id>.fwagent (or pack it as a .fwpack).
//  2. Edit loket.json: set "id" and "name".
//  3. Fill the THREE TODO(connector) functions below: pollPlatform, sendToPlatform,
//     and the env var names in config().
//  4. Build:  GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
//  5. Install via Connections; set the token; enable. Done.
//
// The CORE below (hostFetch, loketCall, forwardToAgent, handle dispatch) is generic
// — you should not need to touch it.
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
	return "connector-template"
}

// ── CORE (generic — do not edit) ─────────────────────────────────────────────

// hostFetch is the one outbound-HTTP primitive, for BOTH the loket and your
// platform's API. Returns (status, body); (0, nil) on host error.
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

// loketCall asks the kernel for a capability — the connector's only door to the
// agent, via bus.request.
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

// forwardToAgent is the dumb-pipe CORE: hand a message to the target agent and
// return its reply text. Platform-agnostic, so handle() can exercise the whole
// connector→bus→agent path with no live platform.
func forwardToAgent(target, text string, chatID int64, user string) string {
	if target == "" {
		target = config().Target
	}
	r, err := loketCall("bus.request", map[string]any{
		"to":      target,
		"payload": map[string]any{"text": text, "chat_id": chatID, "user": user},
	})
	if err != nil {
		return "[connector] agent error: " + err.Error()
	}
	// bus.request → {"reply": <agent emit>}; the agent's emit is {"reply": "...",}.
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
		var s string
		if json.Unmarshal(outer.Reply, &s) == nil && s != "" {
			return s
		}
	}
	return "(agent returned no reply)"
}

// inMsg is one inbound message normalized from your platform.
type inMsg struct {
	Text   string
	ChatID int64  // a conversation/thread id you can reply to
	User   string // sender handle (for logging / allow-listing)
}

// connConfig holds what every connector needs. Token is self-managed (injected from
// this connector's OWN connector.json by the host).
type connConfig struct {
	Token   string
	Target  string
	Allowed map[int64]bool
}

// ── TODO(connector) #1: configuration ────────────────────────────────────────
// Rename the env vars to your platform. The host injects these from this
// connector's own connector.json (Connections → set token + target).
func config() connConfig {
	c := connConfig{
		Token:   strings.TrimSpace(os.Getenv("CONNECTOR_TOKEN")),  // TODO: e.g. DISCORD_BOT_TOKEN
		Target:  strings.TrimSpace(os.Getenv("TARGET_AGENT")),     // which agent to feed
		Allowed: parseAllowed(os.Getenv("CONNECTOR_ALLOWED")),     // optional allow-list
	}
	if c.Target == "" {
		c.Target = "mr-flow-next"
	}
	return c
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

// ── TODO(connector) #2: receive (POLL your platform's HTTP API) ──────────────
// Long-poll or fetch new messages from your platform and return them normalized.
// Return the new "offset/cursor" so the next call continues after these.
// Example below is a stub; replace the URL + parsing with your platform's API.
func pollPlatform(token string, offset int64) (msgs []inMsg, nextOffset int64) {
	// st, raw := hostFetch("GET", "https://api.YOURPLATFORM.com/getMessages?offset="+strconv.FormatInt(offset,10), nil, nil)
	// parse `raw` into []inMsg, set nextOffset past the last message id.
	_ = token
	return nil, offset // TODO: implement
}

// ── TODO(connector) #3: send (POST a reply to your platform's HTTP API) ──────
func sendToPlatform(token string, chatID int64, text string) error {
	// body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	// st, _ := hostFetch("POST", "https://api.YOURPLATFORM.com/sendMessage", map[string]string{"Content-Type":"application/json"}, body)
	// if st == 0 || st >= 400 { return fmt.Errorf("send status=%d", st) }
	_, _, _ = token, chatID, text
	return nil // TODO: implement
}

// ── live daemon (generic — usually no edits needed) ──────────────────────────
// boot polls the platform and relays each allowed message. IDLE (clean exit) with
// no token, so it can sit loaded without stealing another instance's messages.
func boot() {
	c := config()
	if c.Token == "" {
		fmt.Fprintf(os.Stderr, "[%s] no token — connector IDLE (set token in Connections to go live)\n", selfID())
		emit(map[string]any{"ok": true, "status": "idle (no token)"})
		return
	}
	fmt.Fprintf(os.Stderr, "[%s] live: target=%s allowed=%d\n", selfID(), c.Target, len(c.Allowed))
	var offset int64
	for {
		msgs, next := pollPlatform(c.Token, offset)
		offset = next
		for _, m := range msgs {
			if strings.TrimSpace(m.Text) == "" {
				continue
			}
			if len(c.Allowed) > 0 && !c.Allowed[m.ChatID] {
				continue
			}
			reply := forwardToAgent(c.Target, m.Text, m.ChatID, m.User)
			if reply != "" {
				if err := sendToPlatform(c.Token, m.ChatID, reply); err != nil {
					fmt.Fprintf(os.Stderr, "[%s] send: %v\n", selfID(), err)
				}
			}
		}
	}
}

// handleMessage is the TESTABLE core: process ONE message, callable via RPC with a
// synthetic payload so connector→bus→agent is verifiable with no live platform.
func handleMessage(argsJSON string) {
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
		target = config().Target
	}
	reply := forwardToAgent(target, in.Text, in.ChatID, in.User)
	sent := false
	if in.Send {
		if c := config(); c.Token != "" && in.ChatID != 0 {
			sent = sendToPlatform(c.Token, in.ChatID, reply) == nil
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
	case "handle_message", "handle_update":
		handleMessage(args)
	case "handle":
		// Loket-bus invocation — unwrap the Message payload, treat as a message.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		p := args
		if len(msg.Payload) > 0 {
			p = string(msg.Payload)
		}
		handleMessage(p)
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}
