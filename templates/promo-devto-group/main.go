// Package main is the Flowork "promo-devto" group — a per-platform promo colony
// (pasukan-semut) for Dev.to. ONE platform = ONE group. The pipeline is three
// ordered specialist members, then the coordinator publishes:
//
//	1. SEO     → decides the TITLE + KEYWORDS
//	2. WRITER  → writes the article body (weaving the keywords, honest, no hype)
//	3. TAGS    → picks the best Dev.to tags
//	→ coordinator appends the repo links + POSTs to the Dev.to Forem API.
//
// Every article ALWAYS carries both product repo links (Flowork Agent + Flow
// Router) — appended deterministically by the coordinator, not left to the model.
// Reaches every capability through the loket (call(cap,args)); the Forem POST goes
// out via host_net_fetch (the coordinator carries net:fetch:https://dev.to). API
// key + publish flag + tags live in this group's kv or workspace, never hardcoded.
// Members reuse the stock ant wasm (persona-only). Build:
// GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
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
// the transparent way for the owner to drop a secret/setting. "" if absent.
func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// cfg reads a setting from this group's loket kv first (Group Colony menu), then
// falls back to a workspace file of the same name (owner-dropped).
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

// hostFetch makes a raw outbound HTTP call (POST to the Dev.to Forem API).
// Returns (httpStatus, responseBody). Needs net:fetch:https://dev.to in the manifest.
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
	SEO    string
	Writer string
	Tags   string
}

// loadRoster: the three ordered roles. Defaults work out of the box; the Group
// Colony "members" list (in order: seo, writer, tags) or per-role kv keys override.
func loadRoster() roster {
	rs := roster{SEO: "promo-devto-seo", Writer: "promo-devto-writer", Tags: "promo-devto-tags"}
	if v := kvGet("seo_agent"); v != "" {
		rs.SEO = v
	}
	if v := kvGet("writer_agent"); v != "" {
		rs.Writer = v
	}
	if v := kvGet("tags_agent"); v != "" {
		rs.Tags = v
	}
	if m := kvGet("members"); m != "" {
		parts := []string{}
		for _, x := range strings.Split(m, ",") {
			if x = strings.TrimSpace(x); x != "" {
				parts = append(parts, x)
			}
		}
		if len(parts) >= 1 {
			rs.SEO = parts[0]
		}
		if len(parts) >= 2 {
			rs.Writer = parts[1]
		}
		if len(parts) >= 3 {
			rs.Tags = parts[2]
		}
	}
	return rs
}

func kvSet(k, v string) { _, _ = loketCall("store.kv.set", map[string]any{"k": k, "v": v}) }

// brainAdd stores one grounding drawer in this group's OWN brain, tagged by room
// (the topic) so the writer can later pull exactly the facts for that topic.
func brainAdd(content, room string) {
	_, _ = loketCall("store.brain.add", map[string]any{"content": content, "wing": "docs", "room": room})
}

// brainSearch pulls the top-k grounding drawers for a topic — the ONLY facts the
// writer is allowed to use. Returns each drawer's content text.
func brainSearch(query string, k int) []string {
	r, err := loketCall("store.brain.search", map[string]any{"query": query, "k": k})
	if err != nil {
		return nil
	}
	var s struct {
		Hits []struct {
			Content string `json:"content"`
		} `json:"hits"`
	}
	if json.Unmarshal(r, &s) != nil {
		return nil
	}
	out := []string{}
	for _, h := range s.Hits {
		if c := strings.TrimSpace(h.Content); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// splitNonEmpty splits a newline-joined list into trimmed, non-empty entries.
func splitNonEmpty(s string) []string {
	out := []string{}
	for _, x := range strings.Split(s, "\n") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// markPosted records a topic in the dedup ledger (kv "posted_topics", one per
// line) and keeps a human-readable trail (kv "posted_log") of title + url. Called
// only after a confirmed post, so a draft/failure never burns a topic.
func markPosted(topic, title, url string) {
	posted := splitNonEmpty(kvGet("posted_topics"))
	if !contains(posted, topic) {
		posted = append(posted, topic)
		kvSet("posted_topics", strings.Join(posted, "\n"))
	}
	log := strings.TrimSpace(kvGet("posted_log"))
	entry := topic + " | " + title + " | " + url
	if log == "" {
		kvSet("posted_log", entry)
	} else {
		kvSet("posted_log", log+"\n"+entry)
	}
}

// seedFacts ingests grounding docs into this group's brain — one drawer per topic
// (room). It also maintains the kv "topics" backlog (the ordered list of topics
// the group can write about). Re-runnable: brain dedups identical drawers, and
// topics are merged (new ones appended, order preserved). This is how the colony
// learns the REAL product (README + handbook + codemap facts) so the writer never
// has to invent anything.
func seedFacts(argsJSON string) {
	var in struct {
		Items []struct {
			Room    string `json:"room"`
			Content string `json:"content"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		emit(map[string]any{"error": "bad args: " + err.Error()})
		return
	}
	topics := splitNonEmpty(kvGet("topics"))
	seeded := 0
	for _, it := range in.Items {
		room := strings.TrimSpace(it.Room)
		content := strings.TrimSpace(it.Content)
		if room == "" || content == "" {
			continue
		}
		brainAdd(content, room)
		seeded++
		if !contains(topics, room) {
			topics = append(topics, room)
		}
	}
	kvSet("topics", strings.Join(topics, "\n"))
	emit(map[string]any{"group": selfID(), "seeded": seeded, "topics": topics, "topics_total": len(topics)})
}

// autoPost is the autonomous pipeline: pick the next topic the colony hasn't
// covered yet (deterministic anti-duplicate), ground it in the seeded brain
// facts, then run SEO → writer → tags → publish. No source material is passed in
// — the facts come from this group's own brain, so the article is grounded and
// never repeats a topic.
func autoPost() {
	topics := splitNonEmpty(kvGet("topics"))
	if len(topics) == 0 {
		emit(map[string]any{"error": "no topics seeded — run seed_facts first"})
		return
	}
	posted := splitNonEmpty(kvGet("posted_topics"))
	next := ""
	for _, tp := range topics {
		if strings.HasPrefix(tp, "_") { // reserved/test topics (e.g. _ping) are never published
			continue
		}
		if !contains(posted, tp) {
			next = tp
			break
		}
	}
	if next == "" {
		emit(map[string]any{"group": selfID(), "status": "all topics covered",
			"topics_total": len(topics), "posted_total": len(posted),
			"hint": "seed more topics (seed_facts), or clear kv 'posted_topics' to recycle"})
		return
	}

	facts := strings.Join(brainSearch(next, 6), "\n\n---\n\n")
	facts = strings.TrimSpace(facts)
	if facts == "" {
		emit(map[string]any{"error": "no grounding facts in brain for topic '" + next + "' — re-seed", "topic": next})
		return
	}

	rs := loadRoster()

	// 1. SEO — title + keywords, grounded in the facts (no invented angle).
	seoOut := askMember(rs.SEO, "You are an SEO researcher for Dev.to. The article TOPIC is \""+next+"\". Using ONLY the "+
		"FACTS below as the source of truth, decide the best article TITLE (clear, specific, keyword-rich, no clickbait) and "+
		"5-8 KEYWORDS a developer would actually search. Reply EXACTLY in this format and nothing else:\nTITLE: <the title>\n"+
		"KEYWORDS: kw1, kw2, kw3, ...\n\nFACTS:\n"+facts)
	if seoOut == "" {
		emit(map[string]any{"error": "SEO agent (" + rs.SEO + ") gave no output — installed + router up?", "topic": next})
		return
	}
	title := parseField(seoOut, "TITLE")
	keywords := parseField(seoOut, "KEYWORDS")
	if title == "" {
		for _, ln := range strings.Split(seoOut, "\n") {
			if ln = strings.TrimSpace(ln); ln != "" {
				title = strings.TrimLeft(ln, "# ")
				break
			}
		}
	}
	if title == "" {
		emit(map[string]any{"error": "could not parse a TITLE from the SEO agent", "topic": next, "raw": trunc(seoOut, 300)})
		return
	}
	if len(title) > 120 {
		title = title[:120]
	}

	// 2. writer — STRICT grounding: only what the FACTS support (anti-halu).
	body := askMember(rs.Writer, "You are a Dev.to technical writer. Write the article BODY in Markdown about \""+next+"\", "+
		"built around the TITLE and weaving the KEYWORDS in naturally. CRITICAL: use ONLY the FACTS below as your source of "+
		"truth — every claim must be supported by them. If a detail (a feature, number, command, or capability) is NOT in the "+
		"FACTS, do NOT state it; never invent or assume. Be concrete and technical; HONEST — state real strengths plainly, "+
		"acknowledge trade-offs, NEVER overclaim or hype. Output ONLY the Markdown body (do NOT repeat the title as a heading)."+
		"\n\nTITLE: "+title+"\nKEYWORDS: "+keywords+"\n\nFACTS:\n"+facts)
	body = stripLeadingTitle(strings.TrimSpace(body), title)
	if body == "" {
		emit(map[string]any{"error": "writer (" + rs.Writer + ") produced no body", "topic": next})
		return
	}

	// 3. tags + repo footer + publish — records the topic in the ledger on success.
	tagsAndPublish(rs, title, keywords, body, next)
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
			Text    string          `json:"text"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		text := msg.Text
		if len(msg.Payload) > 0 {
			args = string(msg.Payload)
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(msg.Payload, &p)
			text = p.Text
		}
		// Scheduler / owner trigger: a bare "/auto" (or "auto_post") message runs the
		// autonomous pipeline; any other text is treated as article source material.
		if tt := strings.ToLower(strings.TrimSpace(text)); tt == "/auto" || tt == "auto_post" || tt == "auto" {
			autoPost()
		} else {
			runPromo(args)
		}
	case "auto_post":
		// Autonomous: pick the next un-covered topic, ground it in this group's
		// brain (seeded facts), write, and post. No source material needed.
		autoPost()
	case "seed_facts":
		// Ingest grounding docs into this group's brain, one drawer per topic.
		// args: {items:[{room, content}]} — room = the topic key (e.g. "scanner").
		seedFacts(args)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// parseField pulls "FIELD: value" out of a line block (case-insensitive field).
func parseField(s, field string) string {
	up := strings.ToUpper(field) + ":"
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(strings.ToUpper(ln), up) {
			return strings.TrimSpace(ln[len(field)+1:])
		}
	}
	return ""
}

// stripLeadingTitle removes a leading H1 / repeated title line from a body.
func stripLeadingTitle(body, title string) string {
	b := strings.TrimSpace(body)
	if nl := strings.IndexByte(b, '\n'); nl > 0 {
		first := strings.TrimSpace(strings.TrimLeft(b[:nl], "# "))
		if strings.EqualFold(first, title) || strings.HasPrefix(b, "# ") {
			return strings.TrimSpace(b[nl+1:])
		}
	}
	return b
}

// repoFooter is appended to EVERY article — the two products we push, always linked.
const repoFooter = "\n\n---\n\n**Flowork is open source — both products:**\n\n" +
	"- 🤖 **Flowork Agent** (the self-hosted agent OS): https://github.com/flowork-os/Flowork_Agent\n" +
	"- 🛣️ **Flow Router** (the sovereign LLM gateway): https://github.com/flowork-os/flowork_Router\n"

// runPromo: SEO (title+keywords) → writer (body) → tags → append repo links → publish.
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

	// 1. SEO research → title + keywords
	seoOut := askMember(rs.SEO, "You are an SEO researcher for Dev.to. From the SOURCE, decide the best article "+
		"TITLE (clear, specific, keyword-rich, no clickbait) and 5-8 KEYWORDS a developer would actually search. "+
		"Reply EXACTLY in this format and nothing else:\nTITLE: <the title>\nKEYWORDS: kw1, kw2, kw3, ...\n\nSOURCE:\n"+topic)
	if seoOut == "" {
		emit(map[string]any{"error": "SEO agent (" + rs.SEO + ") gave no output — installed + router up?"})
		return
	}
	title := parseField(seoOut, "TITLE")
	keywords := parseField(seoOut, "KEYWORDS")
	if title == "" {
		for _, ln := range strings.Split(seoOut, "\n") {
			if ln = strings.TrimSpace(ln); ln != "" {
				title = strings.TrimLeft(ln, "# ")
				break
			}
		}
	}
	if title == "" {
		emit(map[string]any{"error": "could not parse a TITLE from the SEO agent", "raw": trunc(seoOut, 300)})
		return
	}
	if len(title) > 120 {
		title = title[:120]
	}

	// 2. writer → article body (weaves keywords, honest, no hype)
	body := askMember(rs.Writer, "You are a Dev.to technical writer. Write the article BODY in Markdown for the SOURCE, "+
		"built around the TITLE and weaving the KEYWORDS in naturally. Be concrete and technical; HONEST — state real "+
		"strengths plainly, acknowledge trade-offs, and NEVER overclaim or hype. Output ONLY the Markdown body (do NOT "+
		"repeat the title as a heading).\n\nTITLE: "+title+"\nKEYWORDS: "+keywords+"\n\nSOURCE:\n"+topic)
	body = stripLeadingTitle(strings.TrimSpace(body), title)
	if body == "" {
		emit(map[string]any{"error": "writer (" + rs.Writer + ") produced no body"})
		return
	}

	// 3. tags + repo footer + publish — shared with the autonomous path.
	//    topic="" on the manual path, so nothing is written to the dedup ledger.
	tagsAndPublish(rs, title, keywords, body, "")
}

// tagsAndPublish runs the tags member, appends the two repo links, resolves the
// key (Settings → API Keys first), and POSTs to Dev.to. On the autonomous path
// `topic` is non-empty: a successful post appends it to the dedup ledger so the
// group never writes about the same topic twice.
// shareToFloworkOS posts a published article (title + link) to the FLOWORK_OS
// Telegram group. The group chat id + bot token come from Settings → API Keys
// (FWOS_CHAT_ID / FWOS_BOT_TOKEN, forwarded as env), with a fallback
// to this group's own config — never hardcoded. The bot must be a member of the
// group. Returns (shared, note); note explains why it was skipped/failed.
func shareToFloworkOS(title, url string) (bool, string) {
	chat := strings.TrimSpace(os.Getenv("FWOS_CHAT_ID"))
	if chat == "" {
		chat = cfg("fwos_chat_id")
	}
	token := strings.TrimSpace(os.Getenv("FWOS_BOT_TOKEN"))
	if token == "" {
		token = cfg("fwos_bot_token")
	}
	if chat == "" || token == "" {
		return false, "not configured — set FWOS_CHAT_ID + FWOS_BOT_TOKEN in Settings → API Keys"
	}
	payload, _ := json.Marshal(map[string]any{"chat_id": chat, "text": title + "\n" + url})
	status, resp := hostFetch("POST", "https://api.telegram.org/bot"+token+"/sendMessage",
		map[string]string{"Content-Type": "application/json"}, payload)
	if status >= 200 && status < 300 {
		return true, ""
	}
	return false, trunc(resp, 200)
}

func tagsAndPublish(rs roster, title, keywords, body, topic string) {
	tagsOut := askMember(rs.Tags, "You are a Dev.to tagging specialist. Pick the 4 BEST Dev.to tags for this article — "+
		"lowercase single words from Dev.to's common taxonomy, the most relevant + discoverable. Reply with ONLY the "+
		"tags, comma-separated, nothing else.\n\nTITLE: "+title+"\n\nARTICLE:\n"+trunc(body, 2000))
	tagList := []string{}
	for _, t := range strings.Split(tagsOut, ",") {
		t = strings.ToLower(strings.TrimSpace(strings.Trim(t, "#")))
		t = strings.ReplaceAll(t, " ", "")
		if t != "" && len(tagList) < 4 {
			tagList = append(tagList, t)
		}
	}
	if len(tagList) == 0 {
		tagList = []string{"opensource", "ai", "go", "selfhosted"}
	}

	// always carry both product repo links.
	body = body + repoFooter

	publish := strings.EqualFold(cfg("publish"), "true")
	// API key: primary from Settings → API Keys (env DEVTO_API_KEY, forwarded by the host),
	// fallback to this group's kv / workspace file. Never hardcoded.
	apiKey := strings.TrimSpace(os.Getenv("DEVTO_API_KEY"))
	if apiKey == "" {
		apiKey = cfg("devto_api_key")
	}
	if apiKey == "" {
		emit(map[string]any{
			"group": selfID(), "status": "drafted (NOT posted)",
			"reason":  "devto_api_key not set (Group Colony config or workspace/devto_api_key)",
			"title":   title, "keywords": keywords, "tags": tagList, "would_publish": publish,
			"topic":   topic, "preview": trunc(body, 700),
		})
		return
	}

	article := map[string]any{"title": title, "body_markdown": body, "published": publish, "tags": tagList}
	reqBody, _ := json.Marshal(map[string]any{"article": article})
	status, resp := hostFetch("POST", "https://dev.to/api/articles",
		map[string]string{"Content-Type": "application/json", "api-key": apiKey, "User-Agent": "Flowork-promo-devto"},
		reqBody)
	out := map[string]any{"group": selfID(), "title": title, "keywords": keywords, "tags": tagList,
		"http_status": status, "published": publish, "topic": topic}
	if status >= 200 && status < 300 {
		var r struct {
			URL string `json:"url"`
			ID  int    `json:"id"`
		}
		_ = json.Unmarshal([]byte(resp), &r)
		out["ok"] = true
		out["url"] = r.URL
		out["id"] = r.ID
		// Anti-duplicate: only on a confirmed post, and only on the autonomous
		// path, mark the topic covered so it's never picked again.
		if topic != "" {
			markPosted(topic, title, r.URL)
		}
		// Share the LIVE article to the FLOWORK_OS Telegram group (title + link).
		// Drafts are skipped — the link isn't public yet. Best-effort: a share
		// failure never fails the post.
		if publish && r.URL != "" {
			shared, snote := shareToFloworkOS(title, r.URL)
			out["shared_flowork_os"] = shared
			if !shared && snote != "" {
				out["share_note"] = snote
			}
		}
	} else {
		out["ok"] = false
		out["error"] = trunc(resp, 300)
	}
	emit(out)
}
