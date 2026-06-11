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

// tweetClamp trims a tweet to a safe length. X's hard limit is 280 "weighted"
// chars (emoji/CJK count 2, URLs a fixed 23), so a plain rune count can
// under-estimate. We clamp at 250 runes to leave headroom — the writer is already
// asked for <=270, this is just the backstop.
func tweetClamp(s string) string { return tweetClampN(s, 250) }

// tweetClampN clamps to n runes, rune-safe.
func tweetClampN(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
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
		"Content-Type":              "application/json",
		"Authorization":             "Bearer " + xBearer,
		"x-csrf-token":              ct0,
		"Cookie":                    "auth_token=" + authToken + "; ct0=" + ct0,
		"x-twitter-active-user":     "yes",
		"x-twitter-auth-type":       "OAuth2Session",
		"x-twitter-client-language": "en",
		"Accept":                    "*/*",
		"Accept-Language":           "en-US,en;q=0.9",
		"Origin":                    "https://x.com",
		"Referer":                   "https://x.com/home",
		"User-Agent":                "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
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

// hashtagPrompt — the SEO/hashtag researcher's brief. Reach on X comes from a few
// high-signal tags devs actually follow, not a tag salad.
const hashtagPrompt = "You are a social SEO specialist for developer audiences. Pick the 3 BEST hashtags to maximise " +
	"reach for an X (Twitter) post promoting Flowork — an open-source, self-hosted AI agent OS. Match the TOPIC and the " +
	"POST. Prefer hashtags developers actually search and follow (e.g. opensource, golang, ai, aiagents, selfhosted, " +
	"devtools, llm) — relevance over volume. Reply with ONLY the hashtags, space-separated, lowercase, each starting with " +
	"#, nothing else. Example: #opensource #golang #aiagents"

// hashtagAgent — the SEO member. Default works out of the box; override via kv.
func hashtagAgent() string {
	if v := kvGet("hashtag_agent"); v != "" {
		return v
	}
	return "promo-x-hashtag"
}

// sanitizeHashtags normalises the SEO member's reply into at most `max` clean
// #lowercase tags — defends against tag salad, punctuation, and duplicates.
func sanitizeHashtags(s string, max int) string {
	out := []string{}
	for _, w := range strings.Fields(strings.ReplaceAll(s, ",", " ")) {
		w = strings.TrimLeft(strings.TrimSpace(w), "#")
		w = strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				return r
			default:
				return -1
			}
		}, w)
		if w == "" {
			continue
		}
		tag := "#" + strings.ToLower(w)
		if !contains(out, tag) {
			out = append(out, tag)
		}
		if len(out) >= max {
			break
		}
	}
	return strings.Join(out, " ")
}

// writerAgent — the post writer. Default works out of the box; override via kv
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

// hookPrompt is the X post writer's brief. It encodes the proven viral-hook
// principles (pattern-interrupt opener, conversational tone, ONE concrete idea,
// brevity + impact — lxfater/Awesome-GPTs Viral-Hooks-Generator, AIPIHKAL "open
// with a compelling hook") WITHOUT loosening Flowork's anti-hallucination rule:
// every claim must come from the FACTS. Output is ONE tweet, no link (we append
// the article link as the CTA).
const hookPrompt = "You are a developer-influencer who writes X (Twitter) posts that devs actually stop scrolling for. " +
	"Write ONE post (max 200 characters) about the TOPIC, to make a developer curious about Flowork — a sovereign, " +
	"self-hosted, open-source AI agent OS.\n\n" +
	"How to make it land:\n" +
	"- Open with a PATTERN INTERRUPT: a sharp claim, a 'you don't need X', a myth you break, or a concrete pain devs feel. " +
	"NOT a feature list, NOT a corporate title, NOT 'Introducing…'.\n" +
	"- Conversational, casual, first-person or 'you'. Like telling a smart dev friend, not a press release.\n" +
	"- ONE idea, ONE concrete detail (a real command, number, or capability) — specificity sells.\n" +
	"- Brevity + impact. No hashtag salad (0-2 max). No emoji spam (0-1). Do NOT add a link (it's appended for you).\n" +
	"- HONEST: only what the FACTS support. If it's not in the FACTS, don't claim it. No hype, no overclaim.\n\n" +
	"Study these hooks (different patterns — pick what fits the TOPIC, don't copy verbatim):\n" +
	"- \"Most 'local' AI agents still phone home. Flowork doesn't — one Go binary, offline, your data never leaves the box.\"\n" +
	"- \"You don't need Docker, an account, or the cloud to run AI agents. git clone, ./start.sh, done.\"\n" +
	"- \"Letting an AI agent run code on your machine is terrifying. Unless every app is sandboxed in WASM and asks consent first.\"\n" +
	"- \"Your AI agent OS shouldn't be a black box. Flowork's kernel is ~30 frozen files — everything else is a folder you can read.\"\n\n" +
	"Output ONLY the post text, nothing else."

// composeAndPost is the shared posting path: a writer drafts ONE grounded,
// scroll-stopping post → the coordinator appends the link (the Dev.to article when
// promoting, else the repo) → posts a single tweet → shares it to FLOWORK_OS. A
// single tweet (not a thread) is deliberate: it's far gentler on rate limits and
// never leaves a half-posted thread. `topic` non-empty marks the dedup ledger.
// Returns true if it actually posted.
func composeAndPost(topic, facts, devtoURL string) bool {
	facts = strings.TrimSpace(facts)
	if facts == "" {
		emit(map[string]any{"error": "no grounding facts for this run", "topic": topic})
		return false
	}
	writer := writerAgent()
	what := topic
	if what == "" {
		what = "Flowork"
	}
	hook := strings.TrimSpace(askMember(writer, hookPrompt+"\n\nTOPIC: "+what+"\n\nFACTS:\n"+facts))
	hook = strings.Trim(hook, "\"") // models love wrapping the line in quotes
	if hook == "" {
		emit(map[string]any{"error": "writer (" + writer + ") produced no post — installed + router up?", "topic": topic})
		return false
	}
	hook = tweetClampN(hook, 170) // leave room for the link CTA + hashtags

	// Hashtag/SEO research — a dedicated member picks the tags that maximise reach.
	tags := sanitizeHashtags(askMember(hashtagAgent(), hashtagPrompt+"\n\nTOPIC: "+what+"\n\nPOST: "+hook), 3)

	link := devtoURL
	if link == "" {
		link = "https://github.com/flowork-os/Flowork_Agent"
	}
	tweet := hook + "\n\n👉 " + link
	if tags != "" {
		tweet += "\n" + tags
	}
	tweet = tweetClamp(tweet)

	authToken, ct0 := xCreds()
	if authToken == "" || ct0 == "" {
		emit(map[string]any{
			"group": selfID(), "status": "drafted (NOT posted)", "topic": topic, "devto_url": devtoURL,
			"reason": "X cookies not set — add X_AUTH_TOKEN + X_CT0 in Settings → API Keys",
			"tweet":  tweet,
		})
		return false
	}

	id, status, resp := postTweet(tweet, "", authToken, ct0)
	res := map[string]any{"group": selfID(), "topic": topic, "devto_url": devtoURL, "tweet": tweet}
	posted := id != "" && status >= 200 && status < 300
	if posted {
		url := "https://x.com/i/status/" + id
		res["ok"] = true
		res["url"] = url
		if topic != "" {
			markPosted(topic, hook, url)
		}
		// X posts are NOT shared to Telegram — only Dev.to articles go to the
		// FLOWORK_OS group (promo-devto handles that). X stands on its own feed.
	} else {
		res["ok"] = false
		res["error"] = fmt.Sprintf("post failed (status=%d): %s", status, trunc(resp, 200))
	}
	emit(res)
	return posted
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
	composeAndPost(next, strings.Join(brainSearch(next, 6), "\n\n---\n\n"), "")
}

// promoteDevto asks the sibling promo-devto colony for its most recent article,
// writes a grounded thread about that topic, and drives readers to the Dev.to link.
// Scheduled ~30 min after the Dev.to post so the article is already live. Won't
// promote the same article twice (kv "last_promoted_url").
func promoteDevto() {
	resp := askMember("promo-devto", "/latest")
	var latest struct {
		OK    bool   `json:"ok"`
		Topic string `json:"topic"`
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	if json.Unmarshal([]byte(resp), &latest) != nil || !latest.OK || strings.TrimSpace(latest.URL) == "" {
		emit(map[string]any{"error": "no Dev.to article to promote (asked promo-devto /latest)", "raw": trunc(resp, 200)})
		return
	}
	if kvGet("last_promoted_url") == latest.URL {
		emit(map[string]any{"group": selfID(), "status": "already promoted", "url": latest.URL})
		return
	}
	facts := strings.Join(brainSearch(latest.Topic, 6), "\n\n---\n\n")
	if strings.TrimSpace(facts) == "" {
		facts = latest.Title // fall back to the title if this colony has no seeded facts for the topic
	}
	if composeAndPost(latest.Topic, facts, latest.URL) {
		kvSet("last_promoted_url", latest.URL)
	}
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
	composeAndPost("", src, "")
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
		// Scheduler / owner trigger:
		//   /promote-devto → promote the latest Dev.to article (thread + link)
		//   /auto          → autonomous standalone (this colony's own topic backlog)
		//   <other text>   → a thread from the passed-in source material
		switch tt := strings.ToLower(strings.TrimSpace(text)); {
		case tt == "/promote-devto" || tt == "promote-devto" || tt == "promote_devto":
			promoteDevto()
		case tt == "/auto" || tt == "auto_post" || tt == "auto":
			autoPost()
		default:
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
