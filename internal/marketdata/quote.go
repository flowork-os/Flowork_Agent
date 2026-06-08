// Package marketdata is the "eyes" of the investment team: a small, freeze-safe
// HTTP proxy that fetches real market data so the WASM analyst agents stay dumb
// (they just GET /api/market/quote on localhost — the same way they already call
// the kernel — and never deal with upstream auth).
//
// Right now it sources Yahoo Finance (works for IDX `.JK` tickers and global
// symbols alike), which since 2024 requires a cookie+crumb handshake. That messy
// 2-step auth lives here, server-side, behind one clean JSON endpoint. To add a
// second market later you swap the ticker suffix / add a source — the agents and
// the organ pipeline never change. Non-frozen package; touches no kernel code.
package marketdata

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	uaHeader   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	maxBody    = 4 << 20 // 4 MiB cap on any upstream response
	crumbTTL   = 20 * time.Minute
	httpExpiry = 15 * time.Second
)

// session caches the cookie jar + crumb so we don't re-handshake on every call.
type session struct {
	mu      sync.Mutex
	client  *http.Client
	crumb   string
	fetched time.Time
}

var sess = &session{}

// ensure returns a ready http.Client (with Yahoo cookies) plus a valid crumb,
// refreshing the handshake when the cached crumb is empty or stale.
func (s *session) ensure() (*http.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.crumb != "" && time.Since(s.fetched) < crumbTTL && s.client != nil {
		return s.client, s.crumb, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", err
	}
	c := &http.Client{Timeout: httpExpiry, Jar: jar}
	// Step 1 — land a session cookie.
	if req, e := http.NewRequest(http.MethodGet, "https://fc.yahoo.com", nil); e == nil {
		req.Header.Set("User-Agent", uaHeader)
		if resp, e2 := c.Do(req); e2 == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
			resp.Body.Close()
		}
	}
	// Step 2 — exchange the cookie for a crumb.
	req, err := http.NewRequest(http.MethodGet, "https://query1.finance.yahoo.com/v1/test/getcrumb", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", uaHeader)
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", err
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	crumb := strings.TrimSpace(string(b))
	if crumb == "" || strings.Contains(crumb, "<") {
		return nil, "", errString("could not obtain market crumb")
	}
	s.client, s.crumb, s.fetched = c, crumb, time.Now()
	return c, crumb, nil
}

func (s *session) invalidate() {
	s.mu.Lock()
	s.crumb = ""
	s.mu.Unlock()
}

type errString string

func (e errString) Error() string { return string(e) }

// validTicker keeps the ticker to a safe charset so it can only ever fill a URL
// path/query segment — never smuggle a different host or path (no SSRF).
func validTicker(t string) bool {
	if t == "" || len(t) > 20 {
		return false
	}
	for _, r := range t {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '^' || r == '=':
		default:
			return false
		}
	}
	return true
}

func (s *session) getJSON(u string) (map[string]any, error) {
	c, _, err := s.ensure()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaHeader)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		s.invalidate() // crumb likely expired — next call re-handshakes
		return nil, errString("market upstream rejected auth")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errString("market upstream returned non-JSON")
	}
	return m, nil
}

// raw digs a nested value out of a decoded JSON map by path.
func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

// fnum pulls a number out of Yahoo's {"raw": <num>} wrappers (or a bare number).
func fnum(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case map[string]any:
		if r, ok := x["raw"].(float64); ok {
			return r, true
		}
	}
	return 0, false
}

// Fetch is the shared core: given a ticker (+ optional candle range/interval) it
// returns one normalized map — spot price, OHLCV candles (technical lens) and key
// fundamentals (valuation + financials lenses). Both the HTTP endpoint (human/GUI,
// owner-authed) and the engine `market_quote` tool (agents, via tool.run) call
// THIS, so there is exactly one place that knows the upstream + its auth dance.
// Returns an error only on a bad ticker; upstream hiccups surface as *_error keys
// inside the map so a partial result (price but no fundamentals, say) still flows.
func Fetch(ticker, rng, ivl string) (map[string]any, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if !validTicker(ticker) {
		return nil, errString("invalid ticker")
	}
	if rng == "" {
		rng = "6mo"
	}
	if ivl == "" {
		ivl = "1d"
	}
	// Only the ticker is dynamic in the path; host is fixed. url.PathEscape the
	// ticker for belt-and-suspenders even though validTicker already gates it.
	esc := url.PathEscape(ticker)
	out := map[string]any{"ticker": ticker, "ok": true, "as_of": time.Now().UTC().Format(time.RFC3339)}

	// --- chart: spot + OHLCV ---
	chartURL := "https://query1.finance.yahoo.com/v8/finance/chart/" + esc +
		"?interval=" + url.QueryEscape(ivl) + "&range=" + url.QueryEscape(rng)
	if cm, err := sess.getJSON(chartURL); err == nil {
		res, _ := dig(cm, "chart", "result").([]any)
		if len(res) > 0 {
			r0, _ := res[0].(map[string]any)
			meta, _ := r0["meta"].(map[string]any)
			if meta != nil {
				out["price"], _ = fnum(meta["regularMarketPrice"])
				out["currency"] = meta["currency"]
				out["exchange"] = meta["fullExchangeName"]
			}
			out["ohlcv"] = buildCandles(r0)
		}
	} else {
		out["price_error"] = err.Error()
	}

	// --- fundamentals: valuation + financial health ---
	modules := "summaryDetail,financialData,defaultKeyStatistics"
	if _, crumb, cerr := sess.ensure(); cerr == nil {
		fURL := "https://query1.finance.yahoo.com/v10/finance/quoteSummary/" + esc +
			"?modules=" + url.QueryEscape(modules) + "&crumb=" + url.QueryEscape(crumb)
		if fm, err := sess.getJSON(fURL); err == nil {
			res, _ := dig(fm, "quoteSummary", "result").([]any)
			if len(res) > 0 {
				r0, _ := res[0].(map[string]any)
				out["fundamentals"] = buildFundamentals(r0)
			}
		} else {
			out["fundamentals_error"] = err.Error()
		}
	}
	return out, nil
}

// QuoteHandler serves GET /api/market/quote?ticker=BBCA.JK[&range=3mo&interval=1d]
// for humans / the GUI (owner-authenticated). Agents do NOT use this path (the
// auth middleware gates it); they reach the same data via the market_quote tool.
func QuoteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		out, err := Fetch(r.URL.Query().Get("ticker"), r.URL.Query().Get("range"), r.URL.Query().Get("interval"))
		if err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, out)
	}
}

// buildCandles flattens Yahoo's column-oriented chart into row-oriented OHLCV,
// dropping null gaps (holidays/halts) so the technical lens gets clean bars.
func buildCandles(r0 map[string]any) []map[string]any {
	ts, _ := dig(r0, "timestamp").([]any)
	q, _ := dig(r0, "indicators", "quote").([]any)
	if len(ts) == 0 || len(q) == 0 {
		return nil
	}
	q0, _ := q[0].(map[string]any)
	col := func(name string) []any { v, _ := q0[name].([]any); return v }
	o, h, l, c, v := col("open"), col("high"), col("low"), col("close"), col("volume")
	candles := make([]map[string]any, 0, len(ts))
	for i := range ts {
		cl, ok := fnum(at(c, i))
		if !ok {
			continue // gap bar
		}
		op, _ := fnum(at(o, i))
		hi, _ := fnum(at(h, i))
		lo, _ := fnum(at(l, i))
		vol, _ := fnum(at(v, i))
		tt, _ := fnum(ts[i])
		candles = append(candles, map[string]any{
			"t": int64(tt), "o": op, "h": hi, "l": lo, "c": cl, "v": vol,
		})
	}
	return candles
}

func at(s []any, i int) any {
	if i < 0 || i >= len(s) {
		return nil
	}
	return s[i]
}

// buildFundamentals picks the handful of ratios the valuation + financials lenses
// actually use, normalized out of Yahoo's {raw,fmt} wrappers.
func buildFundamentals(r0 map[string]any) map[string]any {
	sd, _ := r0["summaryDetail"].(map[string]any)
	fd, _ := r0["financialData"].(map[string]any)
	ks, _ := r0["defaultKeyStatistics"].(map[string]any)
	f := map[string]any{}
	set := func(key string, src map[string]any, field string) {
		if src == nil {
			return
		}
		if n, ok := fnum(src[field]); ok {
			f[key] = n
		}
	}
	set("pe", sd, "trailingPE")
	set("forward_pe", sd, "forwardPE")
	set("pb", ks, "priceToBook")
	set("market_cap", sd, "marketCap")
	set("dividend_yield", sd, "dividendYield")
	set("profit_margin", fd, "profitMargins")
	set("operating_margin", fd, "operatingMargins")
	set("roe", fd, "returnOnEquity")
	set("roa", fd, "returnOnAssets")
	set("revenue", fd, "totalRevenue")
	set("revenue_growth", fd, "revenueGrowth")
	set("earnings_growth", fd, "earningsGrowth")
	set("debt_to_equity", fd, "debtToEquity")
	set("current_ratio", fd, "currentRatio")
	set("total_cash", fd, "totalCash")
	set("total_debt", fd, "totalDebt")
	set("free_cashflow", fd, "freeCashflow")
	if fd != nil {
		if rk, ok := fd["recommendationKey"].(string); ok {
			f["analyst_recommendation"] = rk
		}
		if tp, ok := fnum(fd["targetMeanPrice"]); ok {
			f["analyst_target"] = tp
		}
	}
	return f
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
