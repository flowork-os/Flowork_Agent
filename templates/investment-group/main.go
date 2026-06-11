// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// Package main is the Flowork "investment" group — a SEQUENTIAL analyst colony
// with EYES. Unlike the thinking group (pure reasoning), this one first GATHERS
// real data, then reasons on it, so the verdict rests on facts not priors.
//
// Two phases (this is the anti-blind, anti-deadline design):
//
//	PHASE 1 — GATHER (the orchestrator, via the blessed tool.run capability):
//	  • market_quote  → spot price, OHLCV candles, fundamentals (PE/PB/ROE/…)
//	  • web_search    → news/scandal hits on the company + its leadership
//	  • web_archive   → a Wayback snapshot of the top hit (scrubbed article = signal)
//	  All of it is compressed into ONE grounded "data brief".
//
//	PHASE 2 — ANALYZE (member organs, ONE AT A TIME, askMember = done-detector):
//	  financials · valuation · technical · governance · risk  →  decision (synth).
//	Every organ reads the SAME brief — fast, no I/O — and is told to use only those
//	numbers (never fabricate). The decision organ returns INVEST/WATCH/PASS + sizing
//	+ kill-criteria, and always flags "analysis, not financial advice".
//
// IDX-first: a bare symbol defaults to the .JK suffix (BBCA → BBCA.JK). To clone
// this for another market, change the suffix default + news locale — nothing else.
//
// Roster (who plays which organ) is read from this group's OWN loket store, the
// same store the Group Colony menu edits — no hardcoding. Build:
// GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
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

// kvGet reads one key from this group's OWN loket store (live), so roster edits
// from the Group Colony menu apply WITHOUT a restart.
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

// toolRun executes one engine tool by name via the blessed tool.run capability and
// returns its raw `output` (nil on any failure — every caller degrades gracefully,
// so a missing news feed never sinks the whole analysis).
func toolRun(name string, args map[string]any) json.RawMessage {
	r, err := loketCall("tool.run", map[string]any{"name": name, "args": args})
	if err != nil {
		return nil
	}
	var w struct {
		Result struct {
			Output json.RawMessage `json:"output"`
		} `json:"result"`
		Error string `json:"error"`
	}
	if json.Unmarshal(r, &w) != nil || w.Error != "" {
		return nil
	}
	return w.Result.Output
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

type roster struct {
	Organs      []string // analyst organs, run in order (menu "members")
	Synthesizer string   // decision organ
}

// loadRoster reads who plays which organ from this group's OWN loket store (the
// SAME store the Group Colony menu writes). Defaults keep it working out of the box.
func loadRoster() roster {
	rs := roster{
		Organs: []string{
			"investment-financials",
			"investment-valuation",
			"investment-technical",
			"investment-governance",
			"investment-risk",
		},
		Synthesizer: "investment-decision",
	}
	if m := kvGet("members"); m != "" {
		organs := []string{}
		for _, x := range strings.Split(m, ",") {
			if x = strings.TrimSpace(x); x != "" {
				organs = append(organs, x)
			}
		}
		if len(organs) > 0 {
			rs.Organs = organs
		}
	}
	if s := kvGet("synthesizer"); s != "" {
		rs.Synthesizer = s
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
		runInvest(args)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

// tickerRe matches a stock symbol: 2-6 letters, optional ".SUFFIX" (e.g. BBCA.JK,
// AAPL, TLKM). Case-insensitive; we upper-case the catch.
var tickerRe = regexp.MustCompile(`\b([A-Za-z]{2,6})(\.[A-Za-z]{1,3})?\b`)

// stopWords are common English/Indonesian words that look like tickers but aren't,
// so a sentence like "should I buy BBCA" extracts BBCA, not SHOULD.
var stopWords = map[string]bool{
	"SHOULD": true, "WOULD": true, "COULD": true, "ABOUT": true, "WORTH": true,
	"STOCK": true, "SHARE": true, "PRICE": true, "BUY": true, "SELL": true, "HOLD": true,
	"APAKAH": true, "SAHAM": true, "HARGA": true, "BELI": true, "JUAL": true, "LAYAK": true,
	"INVEST": true, "ANALYZE": true, "ANALISA": true, "REVIEW": true, "CURRENT": true,
	// common words that can follow a "saham"/"stock" anchor but are not symbols
	"BAGUS": true, "APA": true, "YANG": true, "MANA": true, "BAIK": true, "INI": true,
	"ITU": true, "LAGI": true, "MURAH": true, "MAHAL": true, "NAIK": true, "TURUN": true,
}

// anchorRe finds a ticker typed right after a "saham/stock/emiten/ticker/kode"
// cue — this catches a CASUAL, lower-case symbol ("analisa saham bbca") that the
// upper-case pass would miss.
var anchorRe = regexp.MustCompile(`(?i)\b(?:saham|stock|emiten|ticker|kode)\s+([A-Za-z]{2,6})\b`)

// extractTicker pulls the most likely ticker out of free text. Preference order:
// (1) an explicit suffixed symbol (BBCA.JK); (2) the word right after a stock cue,
// even lower-case; (3) the first plausible all-caps token. Non-symbols are filtered
// by stopWords; the IDX ".JK" suffix is the local-first default. "" if none found.
func extractTicker(s string) string {
	matches := tickerRe.FindAllStringSubmatch(s, -1)
	// First pass: a symbol that ALREADY carries a market suffix is unambiguous.
	for _, m := range matches {
		if m[2] != "" {
			return strings.ToUpper(m[1]) + strings.ToUpper(m[2])
		}
	}
	// Second pass: the token right after a stock cue word (handles lower-case input).
	if m := anchorRe.FindStringSubmatch(s); m != nil {
		raw := strings.ToUpper(m[1])
		if len(raw) >= 3 && !stopWords[raw] {
			return raw + ".JK"
		}
	}
	// Third pass: an explicitly UPPER-CASE token the user typed as a symbol.
	for _, m := range matches {
		raw := m[1]
		if raw == strings.ToUpper(raw) && !stopWords[strings.ToUpper(raw)] && len(raw) >= 3 {
			return strings.ToUpper(raw) + ".JK" // IDX default (local-first)
		}
	}
	return ""
}

// num renders a JSON number from the fundamentals map compactly (no trailing zeros
// noise), or "n/a" when the key is missing.
func num(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return "n/a"
	}
	f, ok := v.(float64)
	if !ok {
		return fmt.Sprint(v)
	}
	return fmt.Sprintf("%.4g", f)
}

// gatherBrief is PHASE 1: assemble a grounded, compact data brief for the organs.
// Everything is best-effort — a section simply notes "unavailable" rather than
// failing, so the analysis still runs (degraded) when a feed is down.
func gatherBrief(subject string) (string, string) {
	var b strings.Builder
	ticker := extractTicker(subject)

	// --- market data (price + fundamentals + price action) ---
	if ticker != "" {
		b.WriteString("TICKER: " + ticker + "\n")
		if out := toolRun("market_quote", map[string]any{"ticker": ticker, "range": "6mo", "interval": "1d"}); out != nil {
			var q struct {
				Price        float64              `json:"price"`
				Currency     string               `json:"currency"`
				Exchange     string               `json:"exchange"`
				Fundamentals map[string]any       `json:"fundamentals"`
				OHLCV        []map[string]float64 `json:"ohlcv"`
				PriceErr     string               `json:"price_error"`
			}
			if json.Unmarshal(out, &q) == nil {
				if q.Price > 0 {
					b.WriteString(fmt.Sprintf("PRICE: %.2f %s (%s)\n", q.Price, q.Currency, q.Exchange))
				}
				if f := q.Fundamentals; len(f) > 0 {
					b.WriteString("FUNDAMENTALS: PE=" + num(f, "pe") + " fwdPE=" + num(f, "forward_pe") +
						" PB=" + num(f, "pb") + " ROE=" + num(f, "roe") + " ROA=" + num(f, "roa") +
						" profitMargin=" + num(f, "profit_margin") + " revenue=" + num(f, "revenue") +
						" revGrowth=" + num(f, "revenue_growth") + " D/E=" + num(f, "debt_to_equity") +
						" FCF=" + num(f, "free_cashflow") + " mktCap=" + num(f, "market_cap") + "\n")
					if rec, ok := f["analyst_recommendation"].(string); ok {
						b.WriteString("STREET: " + rec + " (target " + num(f, "analyst_target") + ")\n")
					}
				}
				b.WriteString(priceAction(q.OHLCV) + "\n")
			}
		} else {
			b.WriteString("MARKET DATA: unavailable (feed error or unknown ticker)\n")
		}
	} else {
		b.WriteString("TICKER: not detected in the request — fundamentals/technical run on text only.\n")
	}

	// --- governance intel: news/scandal + leadership + political exposure ---
	b.WriteString("\nGOVERNANCE INTEL (verify before claiming; sourced snippets only):\n")
	query := subject
	if ticker != "" {
		query = strings.TrimSuffix(ticker, ".JK") + " " + subject
	}
	topURL := ""
	for _, q := range []string{
		query + " scandal lawsuit fraud investigation",
		query + " CEO president director commissioner political party",
	} {
		if hits := webSearch(q, 3); hits != "" {
			b.WriteString(hits)
			if topURL == "" {
				topURL = firstURL(hits)
			}
		}
	}
	// Wayback the top hit — a scrubbed/edited article is itself a signal.
	if topURL != "" {
		if snap := toolRun("web_archive", map[string]any{"url": topURL}); snap != nil {
			var s struct {
				Available bool   `json:"available"`
				SnapURL   string `json:"snapshot_url"`
				SnapTS    string `json:"snapshot_timestamp"`
			}
			if json.Unmarshal(snap, &s) == nil && s.Available {
				b.WriteString("ARCHIVE: top source has a Wayback snapshot " + s.SnapTS + " (" + s.SnapURL + ")\n")
			}
		}
	}
	return b.String(), ticker
}

// priceAction summarizes the candle series for the technical lens without dumping
// every bar: window return, range, and the last close.
func priceAction(c []map[string]float64) string {
	if len(c) < 2 {
		return "PRICE ACTION: insufficient candle history."
	}
	first := c[0]["c"]
	last := c[len(c)-1]["c"]
	hi, lo := c[0]["h"], c[0]["l"]
	for _, bar := range c {
		if bar["h"] > hi {
			hi = bar["h"]
		}
		if bar["l"] < lo && bar["l"] > 0 {
			lo = bar["l"]
		}
	}
	chg := 0.0
	if first > 0 {
		chg = (last - first) / first * 100
	}
	return fmt.Sprintf("PRICE ACTION (%d bars): change %.1f%%, range %.2f–%.2f, last close %.2f.",
		len(c), chg, lo, hi, last)
}

// webSearch runs the web_search tool and formats the hits as compact lines.
func webSearch(query string, n int) string {
	out := toolRun("web_search", map[string]any{"query": query, "max_results": n})
	if out == nil {
		return ""
	}
	var r struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if json.Unmarshal(out, &r) != nil || len(r.Results) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range r.Results {
		sn := h.Snippet
		if len(sn) > 180 {
			sn = sn[:180]
		}
		b.WriteString("- " + h.Title + " — " + sn + " [" + h.URL + "]\n")
	}
	return b.String()
}

// firstURL pulls the first bracketed [url] out of formatted hit lines.
func firstURL(hits string) string {
	i := strings.LastIndex(hits[:strings.IndexByte(hits, '\n')+1], "[")
	if i < 0 {
		return ""
	}
	rest := hits[i+1:]
	if j := strings.IndexByte(rest, ']'); j > 0 {
		return rest[:j]
	}
	return ""
}

// runInvest is the two-phase pipeline: gather a grounded brief, then run each
// analyst organ over it ONE AT A TIME, then synthesize one decision.
func runInvest(argsJSON string) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	subject := strings.TrimSpace(in.Text)
	if subject == "" {
		emit(map[string]any{"error": "empty subject"})
		return
	}
	rs := loadRoster()

	// PHASE 1 — gather the grounded data brief.
	brief, ticker := gatherBrief(subject)

	// PHASE 2 — each organ analyzes the subject using ONLY the brief's facts.
	// langNote forces the output language to match the user's request (the SUBJECT),
	// so an Indonesian question gets an Indonesian answer — the brief/labels are
	// English scaffolding, not the user's language.
	langNote := "\n\nIMPORTANT: write your answer in the SAME LANGUAGE as the SUBJECT above (if the subject is in Indonesian, answer in Indonesian)."
	organTask := "SUBJECT:\n" + subject + "\n\nVERIFIED DATA BRIEF (use these numbers; do NOT invent figures beyond them):\n" + brief +
		"\n\nAnalyze the subject through your specialist lens. Be concrete, cite the figures above, and flag any data gap honestly." + langNote

	sections := []string{}
	organOut := map[string]string{}
	for _, organ := range rs.Organs {
		ans := askMember(organ, organTask) // blocks until this organ is DONE, then continue
		if ans == "" {
			ans = "(no answer)"
		}
		organOut[organ] = ans
		sections = append(sections, "### "+organ+"\n"+ans)
	}
	combined := strings.Join(sections, "\n\n")

	// Synthesis — the decision organ fuses the lenses into one call.
	if rs.Synthesizer == "" {
		emit(map[string]any{"group": selfID(), "reply": combined, "ticker": ticker, "organs": organOut})
		return
	}
	synthInput := "SUBJECT:\n" + subject + "\n\nVERIFIED DATA BRIEF:\n" + brief +
		"\n\nANALYST FINDINGS:\n\n" + combined +
		"\n\nFuse these into ONE investment decision: a clear final call (INVEST / WATCH / PASS) + brief reasoning + " +
		"suggested position sizing + the kill-criteria that would change your mind. End with: this is analysis, not financial advice." + langNote
	final := askMember(rs.Synthesizer, synthInput)
	if final == "" {
		emit(map[string]any{"group": selfID(), "reply": combined, "ticker": ticker, "organs": organOut, "synth_error": "synthesizer no reply"})
		return
	}
	emit(map[string]any{"group": selfID(), "reply": final, "ticker": ticker, "organs": organOut})
}
