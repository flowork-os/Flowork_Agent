// ⚠️ EDITING A GROUP? READ doc/handbook/menu-group.md FIRST — the group list is auto-derived (any module with kv group=1); slash menu, ask_group, schedules and Mr.Flow all read the SAME synced list; on/off cascades to all members; reset restores from the repo. Do NOT hardcode the roster or kv "groups".
// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// Package main is the Flowork "repo-reviewer" group — the give-value colony. It
// pulls GitHub's trending list, picks a repo it hasn't covered yet (ledger), reads
// the repo's REAL README to ground itself (anti-hallucination), writes an HONEST
// review (what it does, who it's for, strengths AND trade-offs), and posts it to
// Dev.to, X, and the FLOWORK_OS Telegram group. No Flowork pitch inside a review —
// the reviews are clean goodwill; promotion is the promo-* colonies' job. This is
// the 80/20: mostly lift other projects, occasionally talk about ours.
//
// Reaches every capability through the loket. Cookies/keys live in Settings, never
// hardcoded. Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

const respBufBytes = 1 << 20

var outBuf [respBufBytes]byte

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func emit(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }

func selfID() string { return os.Getenv("FLOWORK_AGENT_ID") }

func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func cfg(key string) string {
	if v := kvGet(key); v != "" {
		return v
	}
	return readWS(key)
}

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

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

// hostFetch — raw outbound HTTP. headers optional; returns (status, body).
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

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

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

// ── GitHub trending + README ─────────────────────────────────────────────────

// trendingRe pulls each trending repo's "owner/repo" slug out of the trending
// page. GitHub renders them as <h2 class="h3 lh-condensed"> … <a data-hydro-click="
// {…long json…}" href="/owner/repo"> — the href trails a long attribute, so the
// window has to reach past it (non-greedy grabs the first href after the class).
var trendingRe = regexp.MustCompile(`h3 lh-condensed[\s\S]{0,600}?href="/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)"`)

// fetchTrending returns the trending repo slugs (owner/repo), our own org skipped.
func fetchTrending() []string {
	url := "https://github.com/trending"
	if w := cfg("trending_window"); w != "" { // "daily" | "weekly" | "monthly"
		url += "?since=" + w
	}
	status, body := hostFetch("GET", url, map[string]string{
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Accept":     "text/html",
	}, nil)
	if status < 200 || status >= 300 {
		return nil
	}
	out := []string{}
	for _, m := range trendingRe.FindAllStringSubmatch(body, -1) {
		slug := m[1]
		if strings.HasPrefix(strings.ToLower(slug), "flowork-os/") { // never "review" ourselves
			continue
		}
		if !contains(out, slug) {
			out = append(out, slug)
		}
	}
	return out
}

// fetchReadme returns the repo's README markdown (capped), trying the default
// branch then common branch names. "" if none readable.
func fetchReadme(slug string) string {
	for _, ref := range []string{"HEAD", "main", "master"} {
		for _, name := range []string{"README.md", "readme.md", "README.markdown"} {
			status, body := hostFetch("GET", "https://raw.githubusercontent.com/"+slug+"/"+ref+"/"+name, nil, nil)
			if status >= 200 && status < 300 && strings.TrimSpace(body) != "" {
				if len(body) > 9000 {
					body = body[:9000]
				}
				return body
			}
		}
	}
	return ""
}

// githubMeta pulls real repo metadata (stars, language, topics, last push, …) via
// the flowork-mcp-web MCP tool — Flowork's own Go MCP server — through the loket
// tool.run. This is the MCP edge in action: an agent enriching itself with a tool
// from a connected MCP server. Empty on any failure; the review still works from
// the README alone.
func githubMeta(slug string) string {
	r, err := loketCall("tool.run", map[string]any{"name": "mcp_web_github_repo", "args": map[string]any{"repo": slug}})
	if err != nil {
		return ""
	}
	var w struct {
		Result struct {
			Output json.RawMessage `json:"output"`
		} `json:"result"`
		Output json.RawMessage `json:"output"`
	}
	_ = json.Unmarshal(r, &w)
	raw := w.Result.Output
	if len(raw) == 0 {
		raw = w.Output
	}
	if len(raw) == 0 {
		raw = r // last resort: the whole result
	}
	// MCP tool result shape: {content:[{type,text}]}
	var c struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &c) == nil && len(c.Content) > 0 {
		return strings.TrimSpace(c.Content[0].Text)
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return strings.TrimSpace(s)
	}
	return ""
}

func reviewerAgent() string {
	if v := kvGet("reviewer_agent"); v != "" {
		return v
	}
	return "repo-reviewer-writer"
}

func hashtagAgent() string {
	if v := kvGet("hashtag_agent"); v != "" {
		return v
	}
	return "repo-reviewer-writer"
}

func markReviewed(slug string) {
	rev := splitNonEmpty(kvGet("reviewed"))
	if !contains(rev, slug) {
		rev = append(rev, slug)
		if len(rev) > 300 { // cap the ledger; 300 recent repos is plenty to avoid repeats
			rev = rev[len(rev)-300:]
		}
		kvSet("reviewed", strings.Join(rev, "\n"))
	}
}

// ── X (Twitter) ──────────────────────────────────────────────────────────────

const xBearer = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
const xQueryID = "SiM_cAu83R0wnrpmKQQSEw"
const xCreateTweetURL = "https://x.com/i/api/graphql/" + xQueryID + "/CreateTweet"

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

// postX posts one tweet (browser headers defeat the 226 automation check).
// Returns (url, ok, note).
func postX(text string) (string, bool, string) {
	authToken, ct0 := xCreds()
	if authToken == "" || ct0 == "" {
		return "", false, "X cookies not set"
	}
	vars := map[string]any{
		"tweet_text":              text,
		"dark_request":            false,
		"media":                   map[string]any{"media_entities": []any{}, "possibly_sensitive": false},
		"semantic_annotation_ids": []any{},
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
	if id == "" || status < 200 || status >= 300 {
		return "", false, fmt.Sprintf("status=%d %s", status, trunc(resp, 160))
	}
	return "https://x.com/i/status/" + id, true, ""
}

// ── Dev.to (Forem) ───────────────────────────────────────────────────────────

func postDevto(title, bodyMd string, tags []string) (string, bool, string) {
	apiKey := strings.TrimSpace(os.Getenv("DEVTO_API_KEY"))
	if apiKey == "" {
		apiKey = cfg("devto_api_key")
	}
	if apiKey == "" {
		return "", false, "DEVTO_API_KEY not set"
	}
	publish := strings.EqualFold(cfg("publish"), "true")
	article := map[string]any{"title": title, "body_markdown": bodyMd, "published": publish, "tags": tags}
	reqBody, _ := json.Marshal(map[string]any{"article": article})
	status, resp := hostFetch("POST", "https://dev.to/api/articles",
		map[string]string{"Content-Type": "application/json", "api-key": apiKey, "User-Agent": "Flowork-repo-reviewer"},
		reqBody)
	if status < 200 || status >= 300 {
		return "", false, fmt.Sprintf("status=%d %s", status, trunc(resp, 160))
	}
	var r struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal([]byte(resp), &r)
	return r.URL, true, ""
}

// ── Telegram (FLOWORK_OS) ────────────────────────────────────────────────────

func postTelegram(text string) (bool, string) {
	chat := strings.TrimSpace(os.Getenv("FWOS_CHAT_ID"))
	if chat == "" {
		chat = cfg("fwos_chat_id")
	}
	token := strings.TrimSpace(os.Getenv("FWOS_BOT_TOKEN"))
	if token == "" {
		token = cfg("fwos_bot_token")
	}
	if chat == "" || token == "" {
		return false, "FWOS not configured"
	}
	payload, _ := json.Marshal(map[string]any{"chat_id": chat, "text": text})
	status, resp := hostFetch("POST", "https://api.telegram.org/bot"+token+"/sendMessage",
		map[string]string{"Content-Type": "application/json"}, payload)
	if status >= 200 && status < 300 {
		return true, ""
	}
	return false, trunc(resp, 160)
}

// ── the review ───────────────────────────────────────────────────────────────

const tweetPrompt = "You run a respected developer account that boosts cool open-source projects. Write ONE honest X post " +
	"(max 200 chars) about the repo below, from its README. Make a dev curious: what it does + who it's for, in plain talk. " +
	"Be genuine and SPECIFIC; you may name one honest caveat. No hype, no marketing voice, no link (it's appended). Only the post."

const articlePrompt = "You are a fair, experienced open-source reviewer. From the README, write a short HONEST review of the " +
	"repo (Markdown). Cover: what it is, who it's for, what's genuinely good, an honest trade-off/limitation, and a one-line " +
	"verdict. Be specific and grounded ONLY in the README — never invent features, numbers, or benchmarks. No hype. Reply " +
	"EXACTLY as:\nTITLE: <a clear, non-clickbait title>\n\n<the markdown review body>"

const hashtagPrompt = "Pick the 3 best lowercase hashtags (each starting with #) to help developers discover an X post about " +
	"this open-source repo. Match its real domain/language. Reply with ONLY the hashtags, space-separated."

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
		if !contains(out, tag) && len(out) < max {
			out = append(out, tag)
		}
	}
	return strings.Join(out, " ")
}

// sanitizeHashtagsList — Dev.to wants bare tag words (no #), max 4, lowercase.
func sanitizeHashtagsList(s string) []string {
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
		}, strings.ToLower(w))
		if w != "" && !contains(out, w) && len(out) < 4 {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		out = []string{"opensource", "github", "programming"}
	}
	return out
}

// reviewRepo: pick a fresh trending repo with a README, write an honest review,
// and post it to Dev.to + X + the FLOWORK_OS Telegram group. Reviews are clean —
// no Flowork pitch. Marks the repo reviewed so it's never covered twice.
func reviewRepo() {
	slugs := fetchTrending()
	if len(slugs) == 0 {
		emit(map[string]any{"error": "no trending repos parsed (GitHub markup may have changed)"})
		return
	}
	reviewed := splitNonEmpty(kvGet("reviewed"))
	slug, readme := "", ""
	for _, s := range slugs {
		if contains(reviewed, s) {
			continue
		}
		if rm := fetchReadme(s); rm != "" {
			slug, readme = s, rm
			break
		}
		markReviewed(s) // no README → skip permanently
	}
	if slug == "" {
		emit(map[string]any{"status": "all trending repos already reviewed (or no READMEs)", "trending": len(slugs)})
		return
	}
	repoURL := "https://github.com/" + slug
	owner := strings.SplitN(slug, "/", 2)[0]
	name := slug
	if i := strings.Index(slug, "/"); i >= 0 {
		name = slug[i+1:]
	}
	// Enrich with REAL metadata from the MCP server (stars/language/topics/freshness).
	meta := githubMeta(slug)
	ctx := "REPO: " + slug + "\n"
	if meta != "" {
		ctx += "\nLIVE METADATA (from GitHub):\n" + meta + "\n"
	}
	ctx += "\nREADME:\n" + readme

	// 1. Generate ALL content first (grounded in the README).
	art := askMember(reviewerAgent(), articlePrompt+"\n\n"+ctx)
	title := parseField(art, "TITLE")
	if title == "" {
		title = "An honest look at " + name
	}
	if len(title) > 120 {
		title = title[:120]
	}
	body := strings.TrimSpace(art)
	if idx := strings.Index(body, "\n"); idx >= 0 && strings.HasPrefix(strings.ToUpper(body), "TITLE:") {
		body = strings.TrimSpace(body[idx+1:])
	}
	body += "\n\n---\n\n🔗 Repo: " + repoURL + "\n\n_An honest review by the Flowork team — we read the README so you don't have to. We build open-source tooling too; this isn't a sponsored post._"
	devTags := sanitizeHashtagsList(askMember(hashtagAgent(), hashtagPrompt+"\n\n"+ctx))

	short := strings.Trim(strings.TrimSpace(askMember(reviewerAgent(), tweetPrompt+"\n\n"+ctx)), "\"")
	short = trunc(short, 170)
	htags := sanitizeHashtags(askMember(hashtagAgent(), hashtagPrompt+"\n\n"+ctx), 3)
	tweet := trunc(short+"\n\n👉 "+repoURL+ifs(htags != "", "\n"+htags, ""), 250)

	// Dry run: show the content, post nothing. Set kv/workspace "dry" = "true".
	if strings.EqualFold(cfg("dry"), "true") {
		emit(map[string]any{"group": selfID(), "repo": slug, "dry": true, "mcp_meta": meta,
			"title": title, "dev_tags": devTags, "tweet": tweet, "article": trunc(body, 1200)})
		return
	}

	// 2. Post to all three channels (best-effort each).
	devURL, devOK, devNote := postDevto(title, body, devTags)
	xURL, xOK, xNote := postX(tweet)
	tgOK, tgNote := postTelegram("🔎 Trending on GitHub: " + slug + "\n\n" + short + "\n\n" + repoURL)

	markReviewed(slug)
	emit(map[string]any{
		"group": selfID(), "repo": slug, "owner": owner, "title": title,
		"devto":    map[string]any{"ok": devOK, "url": devURL, "note": devNote},
		"x":        map[string]any{"ok": xOK, "url": xURL, "note": xNote},
		"telegram": map[string]any{"ok": tgOK, "note": tgNote},
	})
}

// ifs is a tiny inline ternary for string assembly.
func ifs(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case "handle_message", "handle", "review", "auto_post":
		// Any trigger (the scheduler sends "/review") runs exactly one review.
		reviewRepo()
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}
