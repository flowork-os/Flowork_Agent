// === STABLE (TESTED) — sengaja BUKAN hard-lock ===
// Status: web_search + html_extract = SCRAPER (markup-dependent). Kalau Mojeek
//   ganti markup, mojeekResultRe/mojeekSnippetRe perlu di-update — jadi file ini
//   boleh di-maintain tanpa "unlock". web_archive = JSON API (stabil).
// Tested 2026-06-02 ke endpoint REAL: web_search 3 hasil, web_archive snapshot
//   ketemu, html_extract teks bersih. Owner: Aola Sahidin (Mr.Dev).
//
// web_research.go — Section 11 / roadmap FASE 3: tools riset biar agent worker
// GA NGARANG sumber. Stdlib-only (no external dep): net/http + regexp +
// html.UnescapeString. Reuse SSRF guard (validateURL) dari web.go (sepaket).
//
// Tools:
//   web_search   — Mojeek HTML endpoint (independen, no API key, ga keblokir
//                  Kominfo — DuckDuckGo diblok di Indonesia). {title,url,snippet}.
//   web_archive  — Wayback Machine availability API. Snapshot terdekat dari URL.
//   html_extract — fetch URL → strip tag → teks readable (buat di-feed ke LLM).
//
// Darkweb di-SKIP (per roadmap: risiko legal + disinfo > nilai).
// pdf_read DEFER (butuh dep PDF parser — keputusan owner).
//
// CAPABILITY: net:fetch:* (worker agent yang butuh subscribe; Mr.Flow ngga).

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/tools"
)

const (
	researchTimeout    = 20 * time.Second
	researchMaxBytes   = 2 * 1024 * 1024 // 2MB cap fetch HTML
	webSearchMaxResult = 8               // anti over-prompt: cap hasil
	htmlExtractMaxText = 12000           // cap teks hasil extract (char)
	// Browser-like UA — sebagian mesin search nolak UA non-browser.
	browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/120.0 Safari/537.36"
)

// researchClient — HTTP client buat tools riset. Timeout + redirect re-validate
// SSRF (reuse validateURL). Beda dari webfetch: ikut redirect tapi tetep di-cek.
var researchClient = &http.Client{
	Timeout: researchTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return fmt.Errorf("too many redirects")
		}
		if _, verr := validateURL(req.URL.String()); verr != nil {
			return fmt.Errorf("redirect blocked: %w", verr)
		}
		req.Header.Del("Authorization")
		return nil
	},
}

// tagRe — strip semua tag HTML.
var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// scriptStyleRe — buang blok <script>/<style> beserta isinya sebelum strip.
var scriptStyleRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)

// wsRe — collapse whitespace beruntun jadi satu.
var wsRe = regexp.MustCompile(`[ \t\f\r]+`)

// blankLinesRe — collapse banyak baris kosong jadi maks dua newline.
var blankLinesRe = regexp.MustCompile(`\n{3,}`)

// stripTags — buang tag HTML + decode entity → teks polos satu baris.
func stripTags(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// =============================================================================
// web_search — Mojeek HTML (independen, no API key)
// =============================================================================
//
// Kenapa Mojeek: DuckDuckGo diblok Kominfo di Indonesia (koneksi di-reset).
// Mojeek = search engine independen (index sendiri), no API key, markup stabil,
// scraper-tolerant, ga keblokir. href hasil = URL asli langsung (no redirect
// wrapper), jadi ga perlu decode.

type webSearchTool struct{}

func (webSearchTool) Name() string       { return "web_search" }
func (webSearchTool) Capability() string { return "net:fetch:*" }
func (webSearchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Web search via Mojeek (no API key, independen). Balikin daftar {title,url,snippet}. Buat cari sumber REAL biar ga ngarang. Cap " + fmt.Sprint(webSearchMaxResult) + " hasil.",
		Params: []tools.Param{
			{Name: "query", Type: tools.ParamString, Description: "kata kunci pencarian", Required: true},
			{Name: "max_results", Type: tools.ParamInt, Description: "jumlah hasil (1-" + fmt.Sprint(webSearchMaxResult) + ", default 5)", Required: false},
		},
		Returns: "{query, count, results:[{title,url,snippet}]}",
	}
}

// mojeekResultRe — anchor judul: <a class="title" ... href="<url-asli>">TITLE</a>.
// href = URL asli langsung (no redirect). TITLE bisa ada nested tag → di-strip.
var mojeekResultRe = regexp.MustCompile(`(?s)<a class="title"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)

// mojeekSnippetRe — snippet per hasil: <p class="s">...</p> (ada <strong> highlight).
var mojeekSnippetRe = regexp.MustCompile(`(?s)<p class="s">(.*?)</p>`)

func (webSearchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	q, _ := args["query"].(string)
	q = strings.TrimSpace(q)
	if q == "" {
		return tools.Result{}, fmt.Errorf("query required")
	}
	limit := 5
	if n, ok := toInt(args["max_results"]); ok && n > 0 {
		limit = n
	}
	if limit > webSearchMaxResult {
		limit = webSearchMaxResult
	}

	endpoint := "https://www.mojeek.com/search?q=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tools.Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html")

	resp, derr := researchClient.Do(req)
	if derr != nil {
		return tools.Result{}, fmt.Errorf("search fetch: %w", derr)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, researchMaxBytes))
	body := string(bodyBytes)

	hrefs := mojeekResultRe.FindAllStringSubmatch(body, -1)
	snips := mojeekSnippetRe.FindAllStringSubmatch(body, -1)

	results := make([]map[string]any, 0, limit)
	for i, m := range hrefs {
		if len(results) >= limit {
			break
		}
		title := stripTags(m[2])
		link := html.UnescapeString(strings.TrimSpace(m[1]))
		if title == "" || link == "" {
			continue
		}
		snippet := ""
		if i < len(snips) {
			snippet = stripTags(snips[i][1])
		}
		results = append(results, map[string]any{
			"title":   title,
			"url":     link,
			"snippet": snippet,
		})
	}

	note := ""
	if len(results) == 0 {
		note = "0 hasil — query terlalu sempit, atau Mojeek lagi rate-limit/berubah markup. Coba kata kunci lain."
	}
	return tools.Result{
		Output: map[string]any{
			"query":   q,
			"count":   len(results),
			"results": results,
		},
		Note: note,
	}, nil
}

// =============================================================================
// web_archive — Wayback Machine availability API
// =============================================================================

type webArchiveTool struct{}

func (webArchiveTool) Name() string       { return "web_archive" }
func (webArchiveTool) Capability() string { return "net:fetch:*" }
func (webArchiveTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Cari snapshot arsip URL di Wayback Machine (archive.org). Buat verifikasi konten lama / sumber yang udah hilang. Balikin snapshot terdekat.",
		Params: []tools.Param{
			{Name: "url", Type: tools.ParamString, Description: "URL yang mau dicari arsipnya", Required: true},
			{Name: "timestamp", Type: tools.ParamString, Description: "opsional, format YYYYMMDD — cari snapshot terdekat ke tanggal ini", Required: false},
		},
		Returns: "{url, available: bool, snapshot_url, snapshot_timestamp, status}",
	}
}

func (webArchiveTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	raw, _ := args["url"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tools.Result{}, fmt.Errorf("url required")
	}
	endpoint := "https://archive.org/wayback/available?url=" + url.QueryEscape(raw)
	if ts, _ := args["timestamp"].(string); strings.TrimSpace(ts) != "" {
		endpoint += "&timestamp=" + url.QueryEscape(strings.TrimSpace(ts))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tools.Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", browserUA)

	resp, derr := researchClient.Do(req)
	if derr != nil {
		return tools.Result{}, fmt.Errorf("archive fetch: %w", derr)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var parsed struct {
		ArchivedSnapshots struct {
			Closest struct {
				Available bool   `json:"available"`
				URL       string `json:"url"`
				Timestamp string `json:"timestamp"`
				Status    string `json:"status"`
			} `json:"closest"`
		} `json:"archived_snapshots"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return tools.Result{}, fmt.Errorf("decode archive response: %w", err)
	}
	c := parsed.ArchivedSnapshots.Closest
	note := ""
	if !c.Available {
		note = "Ga ada snapshot di Wayback buat URL ini."
	}
	return tools.Result{
		Output: map[string]any{
			"url":                raw,
			"available":          c.Available,
			"snapshot_url":       c.URL,
			"snapshot_timestamp": c.Timestamp,
			"status":             c.Status,
		},
		Note: note,
	}, nil
}

// =============================================================================
// html_extract — fetch URL → teks readable (strip tag)
// =============================================================================

type htmlExtractTool struct{}

func (htmlExtractTool) Name() string       { return "html_extract" }
func (htmlExtractTool) Capability() string { return "net:fetch:*" }
func (htmlExtractTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Fetch URL terus ekstrak teks readable (buang script/style/tag). Buat baca artikel/halaman tanpa noise HTML. SSRF guard + cap " + fmt.Sprint(htmlExtractMaxText) + " char.",
		Params: []tools.Param{
			{Name: "url", Type: tools.ParamString, Description: "absolute http(s) URL", Required: true},
		},
		Returns: "{url, title, text, truncated: bool, chars}",
	}
}

// titleRe — ambil <title>.
var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func (htmlExtractTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	raw, _ := args["url"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tools.Result{}, fmt.Errorf("url required")
	}
	u, verr := validateURL(raw) // SSRF guard (reuse dari web.go)
	if verr != nil {
		return tools.Result{}, verr
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return tools.Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html")

	resp, derr := researchClient.Do(req)
	if derr != nil {
		return tools.Result{}, fmt.Errorf("fetch: %w", derr)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, researchMaxBytes))
	raw2 := string(bodyBytes)

	title := ""
	if m := titleRe.FindStringSubmatch(raw2); m != nil {
		title = stripTags(m[1])
	}

	// Buang script/style dulu, baru strip tag sisanya.
	cleaned := scriptStyleRe.ReplaceAllString(raw2, " ")
	cleaned = tagRe.ReplaceAllString(cleaned, "\n")
	cleaned = html.UnescapeString(cleaned)
	cleaned = wsRe.ReplaceAllString(cleaned, " ")
	// rapihin: trim tiap baris, buang baris kosong beruntun.
	var b strings.Builder
	for _, line := range strings.Split(cleaned, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}
	text := blankLinesRe.ReplaceAllString(b.String(), "\n\n")
	text = strings.TrimSpace(text)

	truncated := false
	if len(text) > htmlExtractMaxText {
		text = text[:htmlExtractMaxText]
		truncated = true
	}
	return tools.Result{Output: map[string]any{
		"url":       u.String(),
		"title":     title,
		"text":      text,
		"truncated": truncated,
		"chars":     len(text),
	}}, nil
}

// toInt — best-effort konversi arg numerik (JSON unmarshal → float64) ke int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}
