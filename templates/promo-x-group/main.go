// Package main is the Flowork "promo-x" group — a per-platform promo colony
// (pasukan-semut) for X (Twitter). ONE platform = ONE group. Same colony pattern
// as promo-devto, but it posts a short THREAD to X via the internal GraphQL
// CreateTweet endpoint (cookie auth: X_AUTH_TOKEN + X_CT0 from Settings → API
// Keys). The pipeline:
//
//	1. pick the next un-covered topic (deterministic anti-duplicate ledger)
//	2. ground it in this group's OWN brain (seed_facts: README + handbook)
//	3. a writer member drafts a 3-5 tweet thread, STRICTLY from the facts (no halu)
//	4. the coordinator posts the thread (chained replies) + a closing repo-links tweet
//	5. shares the thread link to the FLOWORK_OS Telegram group
//
// Cookies + group id live in Settings → API Keys, never hardcoded. Members reuse
// the stock ant wasm (persona-only). Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
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

// readWS reads a plain file from this group's OWN folder (mounted at /workspace).
func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// cfg reads a setting from this group's loket kv first, then a workspace file.
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

func kvSet(k, v string) { _, _ = loketCall("store.kv.set", map[string]any{"k": k, "v": v}) }

// askMember sends one subject to a member organ and returns its human reply.
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

// hostFetch makes a raw outbound HTTP call. Returns (httpStatus, responseBody).
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

// brainAdd stores one grounding drawer in this group's OWN brain, tagged by room.
func brainAdd(content, room string) {
	_, _ = loketCall("store.brain.add", map[string]any{"content": content, "wing": "docs", "room": room})
}

// brainSearch pulls the top-k grounding drawers for a topic — the ONLY facts the
// writer is allowed to use.
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

// markPosted records a topic in the dedup ledger + a human-readable trail. Called
// only after a confirmed post, so a failure never burns a topic.
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
// (room) — and maintains the kv "topics" backlog. Re-runnable (brain dedups,
// topics merge). Same store the promo-devto colony uses; seed it the same way.
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

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── X (Twitter) posting ──────────────────────────────────────────────────────

// xBearer — the public web app bearer (same token the X web client ships; not a
// secret, identifies the web app). The per-user identity is the cookie pair.
const xBearer = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
const xQueryID = "SiM_cAu83R0wnrpmKQQSEw"
const xCreateTweetURL = "https://x.com/i/api/graphql/" + xQueryID + "/CreateTweet"

// xFeatures — the feature flags the CreateTweet GraphQL call requires (mirrors the
// X web client; missing flags = 400). If X changes these, a post returns an error
// body that surfaces in the result — never a silent failure.
var xFeatures = map[string]any{
	"creator_subscriptions_tweet_preview_api_enabled":                         true,
	"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
	"tweetypie_unmention_optimization_enabled":                                true,
	"responsive_web_edit_tweet_api_enabled":                                   true,
	"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
	"view_counts_everywhere_api_enabled":                                      true,
	"longform_notetweets_consumption_enabled":                                 true,
	"responsive_web_twitter_article_tweet_consumption_enabled":                true,
	"tweet_awards_web_tipping_enabled":                                        false,
	"longform_notetweets_rich_text_read_enabled":                              true,
	"longform_notetweets_inline_media_enabled":                                true,
	"rweb_video_timestamps_enabled":                                           true,
	"responsive_web_graphql_exclude_directive_enabled":                        true,
	"verified_phone_label_enabled":                                            false,
	"freedom_of_speech_not_reach_fetch_enabled":                               true,
	"standardized_nudges_misinfo":                                             true,
	"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
	"responsive_web_media_download_video_enabled":                             false,
	"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
	"responsive_web_graphql_timeline_navigation_enabled":                      true,
	"responsive_web_enhance_cards_enabled":                                    false,
}

// repoTweet — the closing tweet of every thread: both products, always linked.
const repoTweet = "Flowork is open source 👇\n🤖 Agent OS: https://github.com/flowork-os/Flowork_Agent\n🛣️ Flow Router: https://github.com/flowork-os/flowork_Router\n#opensource #ai #golang #selfhosted"

// xCreds reads the X cookie pair from Settings (env) or this group's own config.
func xCreds() (authToken, ct0 string) {
	authToken = strings.TrimSpace(os.Getenv("X_AUTH_TOKEN"))
	if authToken == "" {
		authToken = cfg("x_auth_token")
	}
	ct0 = strings.TrimSpace(os.Getenv("X_CT0"))
	if ct0 == "" {
		ct0 = cfg("x_ct0")
	}
	return
}

// tweetClamp trims a tweet to X's 280-character (codepoint) limit, rune-safe.
func tweetClamp(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= 280 {
		return s
	}
	return string(r[:279]) + "…"
}

// splitThread turns the writer's reply into individual tweets. Tweets are
// separated by a line containing only "===" (asked for in the prompt). If the
// writer ignores that, the whole reply becomes one clamped tweet.
func splitThread(s string) []string {
	out := []string{}
	cur := []string{}
	flush := func() {
		if t := tweetClamp(strings.Join(cur, "\n")); t != "" {
			out = append(out, t)
		}
		cur = nil
	}
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "===" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return out
}

// postTweet posts ONE tweet (replyTo != "" chains it under a thread). Returns the
// new tweet's rest_id, the http status, and the raw body.
func postTweet(text, replyTo, authToken, ct0 string) (string, int, string) {
	vars := map[string]any{
		"tweet_text":              text,
		"dark_request":            false,
		"media":                   map[string]any{"media_entities": []any{}, "possibly_sensitive": false},
		"semantic_annotation_ids": []any{},
	}
	if replyTo != "" {
		vars["reply"] = map[string]any{"in_reply_to_tweet_id": replyTo, "exclude_reply_user_ids": []any{}}
	}
	body, _ := json.Marshal(map[string]any{"variables": vars, "features": xFeatures, "queryId": xQueryID})
	headers := map[string]string{
		"Content-Type":          "application/json",
		"Authorization":         "Bearer " + xBearer,
		"x-csrf-token":          ct0,
		"Cookie":                "auth_token=" + authToken + "; ct0=" + ct0,
		"x-twitter-active-user": "yes",
		"x-twitter-auth-type":   "OAuth2Session",
	}
	status, resp := hostFetch("POST", xCreateTweetURL, headers, body)
	var parsed struct {
		Data struct {
			CreateTweet struct {
				TweetResults struct {
					Result struct {
						RestID string `json:"rest_id"`
					} `json:"result"`
				} `json:"tweet_results"`
			} `json:"create_tweet"`
		} `json:"data"`
	}
	id := ""
	if json.Unmarshal([]byte(resp), &parsed) == nil {
		id = parsed.Data.CreateTweet.TweetResults.Result.RestID
	}
	return id, status, resp
}

// postThread posts each tweet, chaining replies. Stops on the first failure and
// reports how many landed (so a half-posted thread is visible, not silent).
func postThread(tweets []string, authToken, ct0 string) (firstID string, posted int, errNote string) {
	replyTo := ""
	for i, t := range tweets {
		if t = strings.TrimSpace(t); t == "" {
			continue
		}
		id, status, resp := postTweet(t, replyTo, authToken, ct0)
		if status < 200 || status >= 300 || id == "" {
			return firstID, posted, fmt.Sprintf("tweet %d failed (status=%d): %s", i+1, status, trunc(resp, 220))
		}
		if firstID == "" {
			firstID = id
		}
		replyTo = id
		posted++
	}
	return firstID, posted, ""
}

// shareToFloworkOS posts the published thread (hook + link) to the FLOWORK_OS
// Telegram group. Group chat id + bot token come from Settings → API Keys
// (FWOS_CHAT_ID / FWOS_BOT_TOKEN), fallback to this group's own config. The bot
// must be a member of the group. Returns (shared, note).
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

// writerAgent — the thread writer. Default works out of the box; override via kv
// "writer_agent" or the first entry of the "members" list.
func writerAgent() string {
	if v := kvGet("writer_agent"); v != "" {
		return v
	}
	if m := kvGet("members"); m != "" {
		for _, x := range strings.Split(m, ",") {
			if x = strings.TrimSpace(x); x != "" {
				return x
			}
		}
	}
	return "promo-x-writer"
}

// composeAndPost is the shared posting path: writer drafts a grounded thread →
// post it to X → append the repo tweet → share the link to FLOWORK_OS. `topic` is
// non-empty only on the autonomous path → a confirmed post marks the dedup ledger.
func composeAndPost(topic, facts string) {
	facts = strings.TrimSpace(facts)
	if facts == "" {
		emit(map[string]any{"error": "no grounding facts for this run", "topic": topic})
		return
	}
	writer := writerAgent()
	what := topic
	if what == "" {
		what = "Flowork"
	}
	out := askMember(writer, "You write engaging X (Twitter) threads to promote Flowork — a sovereign, self-hosted AI "+
		"agent OS. Write a SHORT thread (3-5 tweets) about \""+what+"\", built ONLY from the FACTS below. Rules: each tweet "+
		"<= 270 characters; concrete + punchy; tweet 1 is a strong hook (no \"a thread:\" filler); HONEST — real strengths, "+
		"acknowledge trade-offs, NEVER overclaim or hype; if a detail (feature, number, command) is NOT in the FACTS, do NOT "+
		"state it. Separate each tweet with a line containing only ===. Do NOT number the tweets. Output ONLY the tweets.\n\n"+
		"TOPIC: "+what+"\n\nFACTS:\n"+facts)
	tweets := splitThread(out)
	if len(tweets) == 0 {
		emit(map[string]any{"error": "writer (" + writer + ") produced no thread — installed + router up?", "topic": topic})
		return
	}
	tweets = append(tweets, repoTweet) // always close with both product links

	authToken, ct0 := xCreds()
	if authToken == "" || ct0 == "" {
		emit(map[string]any{
			"group": selfID(), "status": "drafted (NOT posted)", "topic": topic,
			"reason": "X cookies not set — add X_AUTH_TOKEN + X_CT0 in Settings → API Keys",
			"tweets": tweets,
		})
		return
	}

	firstID, n, errNote := postThread(tweets, authToken, ct0)
	res := map[string]any{"group": selfID(), "topic": topic, "tweets_posted": n}
	if firstID != "" && errNote == "" {
		url := "https://x.com/i/status/" + firstID
		res["ok"] = true
		res["url"] = url
		if topic != "" {
			markPosted(topic, tweets[0], url)
		}
		shared, snote := shareToFloworkOS(trunc(tweets[0], 120), url)
		res["shared_flowork_os"] = shared
		if !shared && snote != "" {
			res["share_note"] = snote
		}
	} else {
		res["ok"] = false
		res["error"] = errNote
	}
	emit(res)
}

// autoPost — autonomous: pick the next un-covered topic, ground it in the seeded
// brain, draft + post a thread. No source material needed.
func autoPost() {
	topics := splitNonEmpty(kvGet("topics"))
	if len(topics) == 0 {
		emit(map[string]any{"error": "no topics seeded — run seed_facts first"})
		return
	}
	posted := splitNonEmpty(kvGet("posted_topics"))
	next := ""
	for _, tp := range topics {
		if strings.HasPrefix(tp, "_") { // reserved/test topics are never published
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
	composeAndPost(next, strings.Join(brainSearch(next, 6), "\n\n---\n\n"))
}

// runPromo — manual: thread from passed-in source material ({text}). Not added to
// the dedup ledger (topic is empty).
func runPromo(argsJSON string) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	src := strings.TrimSpace(in.Text)
	if src == "" {
		emit(map[string]any{"error": "empty source — pass {\"text\":\"<material>\"} or use /auto"})
		return
	}
	composeAndPost("", src)
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
		// Scheduler / owner trigger: a bare "/auto" message runs the autonomous
		// pipeline; any other text is treated as source material for a thread.
		if tt := strings.ToLower(strings.TrimSpace(text)); tt == "/auto" || tt == "auto_post" || tt == "auto" {
			autoPost()
		} else {
			runPromo(args)
		}
	case "auto_post":
		autoPost()
	case "seed_facts":
		seedFacts(args)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}
