// Package main is the Flowork "promo-devto" group — a per-platform promo colony
// (pasukan-semut) for Dev.to. ONE platform = ONE group: the writer drafts an
// article from source material (changelog/features), the honesty editor strips any
// overclaim, and the coordinator publishes it to Dev.to via the Forem API.
//
// Reaches every capability through the single kernel counter (the loket) via
// call(cap, args), exactly like the investment group. The Forem POST goes out via
// the same host_net_fetch the loket uses — the coordinator carries net:fetch:* so
// it may reach dev.to directly. The Dev.to API key + publish flag + tags live in
// THIS group's own loket kv store (set from the Group Colony menu), never hardcoded.
//
// Members reuse the stock ant wasm (same binary, different persona file) — only this
// coordinator is custom. Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

const respBufBytes = 524288

var outBuf [respBufBytes]byte

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

func selfID() string { return os.Getenv("FLOWORK_AGENT_ID") }

// readWS reads a plain file from this group's OWN folder (mounted at /workspace) —
// the transparent, editable way for the owner to drop a secret/setting without a
// menu. Used as a fallback for the Dev.to API key + publish flag + tags. "" if absent.
func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// cfg reads a setting from this group's loket kv first (Group Colony menu), then
// falls back to a workspace file of the same name (owner-dropped). Empty if neither.
func cfg(key string) string {
	if v := kvGet(key); v != "" {
		return v
	}
	return readWS(key)
}

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

// loketCall is the one door: ask the kernel for a capability by name.
func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})
	reqJSON, _ := json.Marshal(map[string]any{
		"method": "POST", "url": loketURL, "timeout_ms": 120000, "max_resp_bytes": 4 << 20,
		"headers":     map[string]string{"Content-Type": "application/json"},
		"body_base64": base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return nil, fmt.Errorf("host_net_fetch: 0 bytes")
	}
	var host struct {
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:n], &host); err != nil {
		return nil, err
	}
	if host.Error != "" {
		return nil, fmt.Errorf("host: %s", host.Error)
	}
	raw, _ := base64.StdEncoding.DecodeString(host.BodyB64)
	var res struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, fmt.Errorf("loket refused: %s", res.Error)
	}
	return res.Result, nil
}

// kvGet reads one key from this group's OWN loket store (live — Group Colony edits
// apply without a restart).
func kvGet(k string) string {
	r, err := loketCall("store.kv.get", map[string]any{"k": k})
	if err != nil {
		return ""
	}
	var s struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(r, &s) != nil {
		return ""
	}
	return strings.TrimSpace(s.Value)
}

// askMember sends one subject to a member organ and returns its human reply,
// unwrapping the bus.request {reply:{reply:"…"}} double envelope. "" on failure.
func askMember(to, subject string) string {
	r, err := loketCall("bus.request", map[string]any{
		"to": to, "type": "task", "payload": map[string]any{"text": subject},
	})
	if err != nil {
		return ""
	}
	var outer struct {
		Reply json.RawMessage `json:"reply"`
	}
	if json.Unmarshal(r, &outer) != nil || len(outer.Reply) == 0 {
		return ""
	}
	var inner struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal(outer.Reply, &inner) == nil && inner.Reply != "" {
		return inner.Reply
	}
	return string(outer.Reply)
}

// hostFetch makes a raw outbound HTTP call (used to POST to the Dev.to Forem API).
// Returns (httpStatus, responseBody). Needs net:fetch:* in the manifest.
func hostFetch(method, url string, headers map[string]string, body []byte) (int, string) {
	reqJSON, _ := json.Marshal(map[string]any{
		"method": method, "url": url, "timeout_ms": 30000, "max_resp_bytes": 1 << 20,
		"headers":     headers,
		"body_base64": base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return 0, "host_net_fetch: 0 bytes"
	}
	var host struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(outBuf[:n], &host) != nil {
		return 0, "host decode error"
	}
	if host.Error != "" {
		return 0, "host: " + host.Error
	}
	b, _ := base64.StdEncoding.DecodeString(host.BodyB64)
	return host.Status, string(b)
}

type roster struct {
	Writer    string
	Honesty   string
}

func loadRoster() roster {
	rs := roster{Writer: "promo-devto-writer", Honesty: "promo-devto-honesty"}
	if m := kvGet("members"); m != "" {
		for _, x := range strings.Split(m, ",") {
			if x = strings.TrimSpace(x); x != "" {
				rs.Writer = x
				break
			}
		}
	}
	if s := kvGet("synthesizer"); s != "" {
		rs.Honesty = s
	}
	return rs
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch os.Args[1] {
	case "handle_message", "handle":
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) > 0 {
			args = string(msg.Payload)
		}
		runPromo(args)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

func splitTitleBody(s string) (string, string) {
	s = strings.TrimSpace(s)
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return strings.TrimSpace(strings.TrimLeft(s, "# ")), ""
	}
	title := strings.TrimSpace(strings.TrimLeft(s[:nl], "# "))
	body := strings.TrimSpace(s[nl+1:])
	if len(title) > 120 {
		title = title[:120]
	}
	return title, body
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// runPromo: writer drafts → honesty editor finalizes → publish to Dev.to (or, if no
// API key is configured yet, return the draft so the owner can review).
func runPromo(argsJSON string) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	topic := strings.TrimSpace(in.Text)
	if topic == "" {
		emit(map[string]any{"error": "empty topic — pass {\"text\": \"<source material>\"}"})
		return
	}
	rs := loadRoster()

	writeTask := "Write a Dev.to article for a developer audience from the SOURCE below. " +
		"Output the TITLE on the first line, then a blank line, then the article body in Markdown. " +
		"Be concrete, technical, and HONEST — highlight real strengths, never overclaim, no hype.\n\nSOURCE:\n" + topic
	draft := askMember(rs.Writer, writeTask)
	if draft == "" {
		emit(map[string]any{"error": "writer (" + rs.Writer + ") produced no draft — is it installed + the router up?"})
		return
	}

	final := draft
	if rs.Honesty != "" {
		edited := askMember(rs.Honesty, "You are the honesty editor. Edit this Dev.to draft: remove any overclaim, "+
			"hype, or unverifiable superlative; keep it truthful, concrete, and engaging (real strengths stated plainly). "+
			"Keep the SAME format: TITLE on the first line, blank line, then the Markdown body.\n\nDRAFT:\n"+draft)
		if edited != "" {
			final = edited
		}
	}

	title, body := splitTitleBody(final)
	if title == "" || body == "" {
		emit(map[string]any{"error": "could not parse title/body from the editor output", "raw": trunc(final, 400)})
		return
	}

	tagList := []string{}
	for _, t := range strings.Split(cfg("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tagList = append(tagList, t)
		}
	}
	if len(tagList) == 0 {
		tagList = []string{"opensource", "ai", "go", "selfhosted"}
	}
	publish := strings.EqualFold(cfg("publish"), "true")
	article := map[string]any{"title": title, "body_markdown": body, "published": publish, "tags": tagList}

	apiKey := cfg("devto_api_key")
	if apiKey == "" {
		emit(map[string]any{
			"group": selfID(), "status": "drafted (NOT posted)",
			"reason":  "devto_api_key not set in this group's config (Group Colony → config)",
			"title":   title, "tags": tagList, "would_publish": publish,
			"preview": trunc(body, 600),
		})
		return
	}

	reqBody, _ := json.Marshal(map[string]any{"article": article})
	status, resp := hostFetch("POST", "https://dev.to/api/articles",
		map[string]string{"Content-Type": "application/json", "api-key": apiKey, "User-Agent": "Flowork-promo-devto"},
		reqBody)
	out := map[string]any{"group": selfID(), "title": title, "http_status": status, "published": publish}
	if status >= 200 && status < 300 {
		var r struct {
			URL string `json:"url"`
			ID  int    `json:"id"`
		}
		_ = json.Unmarshal([]byte(resp), &r)
		out["ok"] = true
		out["url"] = r.URL
		out["id"] = r.ID
	} else {
		out["ok"] = false
		out["error"] = trunc(resp, 300)
	}
	emit(out)
}
