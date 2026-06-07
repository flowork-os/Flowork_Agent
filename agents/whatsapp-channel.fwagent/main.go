// === LOCKED FILE ===
// Status: STABLE — live-proven (tested via realistic mock harness + live Telegram token 2026-06-07).
// Plug-and-play connector: to change behavior swap the folder; do not hand-edit without owner approval.
// Owner: Aola Sahidin (Mr.Dev). Locked: 2026-06-07.

// Package main is the Flowork WhatsApp CHANNEL — a loket-native adapter over the
// Meta WhatsApp Cloud API.
//
// A channel is a DUMB PIPE (roadmap §F): it owns no brains. It receives WhatsApp
// messages, hands each to a target agent over the loket bus (bus.request), and
// sends the agent's reply back via the Cloud API. ALL the thinking — LLM, tools,
// routing, memory — lives in the agent, never here.
//
// WHY WEBHOOK, NOT POLLING: the WhatsApp Cloud API has NO inbound polling endpoint
// (unlike Telegram getUpdates / Discord+Slack REST history). Inbound messages are
// PUSHED to a webhook. wasm can't listen for HTTP, so delivery rides the host's
// existing, non-frozen webhook intake: Meta POSTs to
//   /api/kernel/webhook/whatsapp-channel?secret=<webhook_secret>
// the host checks the secret against THIS connector's own store and invokes us over
// the bus with the raw Meta payload. We self-provision that secret into our own
// store on boot (store.kv.set), so enabling the connector is pure config — zero
// host edits, zero kernel edits. Outbound replies go to the Cloud API directly.
//
// With no token/secret the connector is IDLE (boot exits clean) so it loads safely
// beside the other channels. The owner sets the token + webhook secret to go live.
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
	return "whatsapp-channel"
}

// hostFetch is the one outbound-HTTP primitive, used for BOTH the loket and the
// Cloud API. Returns (status, body); (0, nil) on host error.
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

// loketCall asks the kernel for a capability by name (the channel's door to the
// agent via bus.request, and to its own store via store.kv.*).
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
// over the loket bus and return the agent's reply text. It is WhatsApp-agnostic, so
// handle_update can exercise the whole channel→bus→agent path with no live token.
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
		var s string
		if json.Unmarshal(outer.Reply, &s) == nil && s != "" {
			return s
		}
	}
	return "(agent ga balikin reply)"
}

// waSession turns a WhatsApp sender id (a phone number string) into a stable
// positive int64 so each contact gets its own agent session key (chat_id).
func waSession(from string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(from))
	return int64(h.Sum64() >> 1)
}

// ── WhatsApp Cloud API I/O (only used live, when a token is set) ─────────────

const graphAPI = "https://graph.facebook.com/v21.0"

func phoneNumberID() string { return strings.TrimSpace(os.Getenv("WHATSAPP_PHONE_NUMBER_ID")) }

// sendMessage posts a text reply to a recipient over the Cloud API.
func sendMessage(token, phoneID, to, text string) error {
	if len(text) > 4000 { // WhatsApp text body limit is 4096
		text = text[:4000] + "\n…(truncated)"
	}
	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]any{"body": text},
	})
	url := fmt.Sprintf("%s/%s/messages", graphAPI, phoneID)
	st, _ := hostFetch("POST", url, map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}, body)
	if st == 0 || st >= 400 {
		return fmt.Errorf("whatsapp send status=%d", st)
	}
	return nil
}

// metaWebhook is the slice of the Meta WhatsApp webhook payload we care about.
type metaWebhook struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					From string `json:"from"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// handleWebhook parses ONE Meta payload and, for each inbound text message,
// forwards it to the agent and replies via the Cloud API. Returns a summary so the
// host intake (and tests) can see what happened. send=false when no token, so the
// parse→forward path is fully testable offline.
func handleWebhook(payloadJSON string) {
	var wh metaWebhook
	if err := json.Unmarshal([]byte(payloadJSON), &wh); err != nil {
		emit(map[string]any{"ok": false, "error": "parse webhook: " + err.Error()})
		return
	}
	token := strings.TrimSpace(os.Getenv("WHATSAPP_TOKEN"))
	target := targetAgent()
	handled := 0
	sent := 0
	var lastReply string
	for _, e := range wh.Entry {
		for _, c := range e.Changes {
			phoneID := phoneNumberID()
			if phoneID == "" {
				phoneID = c.Value.Metadata.PhoneNumberID
			}
			for _, m := range c.Value.Messages {
				if m.Type != "text" || strings.TrimSpace(m.Text.Body) == "" {
					continue // skip non-text (image/audio/status) for this MVP
				}
				handled++
				reply := forwardToAgent(target, m.Text.Body, waSession(m.From), m.From)
				lastReply = reply
				if reply != "" && token != "" && phoneID != "" {
					if err := sendMessage(token, phoneID, m.From, reply); err != nil {
						fmt.Fprintf(os.Stderr, "[%s] send: %v\n", selfID(), err)
					} else {
						sent++
					}
				}
			}
		}
	}
	emit(map[string]any{"ok": true, "handled": handled, "sent": sent, "target": target, "last_reply": lastReply})
}

// handleUpdate processes ONE synthetic message — the simple testable core, mirrors
// the other channels. forwardToAgent is WhatsApp-agnostic so this verifies the
// channel→bus→agent path with no webhook payload.
func handleUpdate(argsJSON string) {
	var in struct {
		Text   string `json:"text"`
		From   string `json:"from"`
		User   string `json:"user"`
		Target string `json:"target"`
		Send   bool   `json:"send"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	target := in.Target
	if target == "" {
		target = targetAgent()
	}
	user := in.User
	if user == "" {
		user = in.From
	}
	reply := forwardToAgent(target, in.Text, waSession(in.From), user)
	sent := false
	if in.Send {
		token := strings.TrimSpace(os.Getenv("WHATSAPP_TOKEN"))
		if token != "" && phoneNumberID() != "" && in.From != "" {
			sent = sendMessage(token, phoneNumberID(), in.From, reply) == nil
		}
	}
	emit(map[string]any{"reply": reply, "target": target, "sent": sent})
}

// boot self-provisions the webhook secret into this connector's OWN store so the
// host intake (/api/kernel/webhook/whatsapp-channel) accepts Meta's POST. WhatsApp
// is webhook-driven — there is no polling loop — so boot does this once and exits.
func boot() {
	secret := strings.TrimSpace(os.Getenv("WHATSAPP_WEBHOOK_SECRET"))
	if secret == "" {
		fmt.Fprintf(os.Stderr, "[%s] no WHATSAPP_WEBHOOK_SECRET — channel IDLE (owner sets token + webhook secret to go live)\n", selfID())
		emit(map[string]any{"ok": true, "status": "idle (no webhook secret)"})
		return
	}
	if _, err := loketCall("store.kv.set", map[string]any{"k": "webhook_secret", "v": secret}); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] self-provision webhook_secret failed: %v\n", selfID(), err)
		emit(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	fmt.Fprintf(os.Stderr, "[%s] live: webhook secret provisioned, awaiting Meta deliveries\n", selfID())
	emit(map[string]any{"ok": true, "status": "armed (webhook secret provisioned)"})
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
	case "handle_webhook":
		handleWebhook(args)
	case "handle":
		// Loket-bus / webhook-intake invocation. The host intake wraps the Meta
		// payload as Message{type:"webhook", payload:<meta json>}; the bus wraps a
		// plain {payload:...}. Unwrap one level, then route by shape: a Meta
		// webhook has "entry" → handleWebhook; otherwise a synthetic update.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		p := args
		if len(msg.Payload) > 0 {
			p = string(msg.Payload)
		}
		if strings.Contains(p, "\"entry\"") {
			handleWebhook(p)
		} else {
			handleUpdate(p)
		}
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}
