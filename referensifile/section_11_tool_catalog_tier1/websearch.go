package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/safeclient"
)

var (
	ddgResultRe  = regexp.MustCompile(`(?s)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>.*?<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)
	htmlEntityRe = regexp.MustCompile(`&[a-zA-Z]+;`)
)

// WebSearchTool — DuckDuckGo HTML search (no API key needed).
type WebSearchTool struct {
	client     *http.Client
	maxResults int
}

type webSearchArgs struct {
	Query string `json:"query" validate:"required"`
	Limit int    `json:"limit,omitempty"`
}

func NewWebSearchTool() *WebSearchTool {
	// BUG-H12 fix (2026-04-19): TIDAK PERNAH skip TLS verification.
	// Sebelumnya `InsecureSkipVerify: true` dipakai sebagai workaround
	// ECONNRESET DuckDuckGo — itu solusi yang SALAH (MITM attack surface,
	// data manipulation, credential exfiltration via Authorization header).
	//
	// Proper fix: ECONNRESET DDG sudah handled via multi-endpoint fallback
	// chain (DDG → SearXNG → Brave → OpenRouter :online) di rc70.
	// TLS verification sekarang default-strict di safeclient.
	return &WebSearchTool{
		client:     safeclient.NewClient(safeclient.DefaultAPITimeout),
		maxResults: 10,
	}
}

func (t *WebSearchTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "websearch",
		Description: "Search the web via DuckDuckGo. Returns up to 10 results (title + snippet + URL).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query string.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (1-10, default 5).",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args webSearchArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode websearch arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Query) == "" {
		return Result{}, fmt.Errorf("query is required")
	}
	limit := args.Limit
	if limit <= 0 || limit > t.maxResults {
		limit = 5
	}

	// rc70 + 2026-05-04 update: Ayah's network blocks DuckDuckGo (ECONNRESET).
	// Try multiple endpoints in order, first one that returns parseable wins.
	// Public SearXNG instances community-run, list sorted by observed reliability
	// (refresh per searx.space saat banyak yang dead).
	q := url.QueryEscape(args.Query)
	endpoints := []string{
		// SearXNG JSON-format (struktur jelas, parser efficient)
		"https://searx.be/search?q=" + q + "&format=json",
		"https://searx.tiekoetter.com/search?q=" + q + "&format=json",
		"https://priv.au/search?q=" + q + "&format=json",
		"https://search.privacyguides.net/search?q=" + q + "&format=json",
		// DDG HTML fallback
		"https://html.duckduckgo.com/html/?q=" + q,
		"https://lite.duckduckgo.com/lite/?q=" + q,
		// Brave HTML last (anti-bot strict)
		"https://search.brave.com/search?q=" + q,
	}

	var lastErr error
	var body []byte
	var winningEndpoint string
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9")

		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("fetch %s: %w", endpoint, err)
			continue
		}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		if rerr != nil {
			lastErr = fmt.Errorf("read %s: %w", endpoint, rerr)
			continue
		}
		if resp.StatusCode >= 400 || len(b) < 200 {
			lastErr = fmt.Errorf("%s returned HTTP %d / %d bytes", endpoint, resp.StatusCode, len(b))
			continue
		}
		body = b
		winningEndpoint = endpoint
		break
	}
	if body == nil {
		// 2026-05-05 fix (Ayah Telegram test): network Ayah block semua HTTP
		// endpoint (DDG/SearXNG/Brave) → context deadline exceeded. Daripada
		// kasih balik error message yang bikin warga halu lapor keluh-kesah,
		// auto-delegate ke browser_search (Rod-based, Bing primary fallback
		// chain — udah verified jalan di network Ayah).
		//
		// Per STANDAR_KERJA §1.8 API-FREE PRINCIPLE: browser_search itu
		// sovereign (no API key, full HTML render via Chromium), bukan 3rd
		// party API → match doctrine.
		bs := NewBrowserSearchTool()
		bsArgs, _ := json.Marshal(map[string]any{"query": args.Query})
		bsResult, bsErr := bs.Execute(ctx, Invocation{
			ToolName:   "browser_search",
			Arguments:  bsArgs,
			ParsedArgs: map[string]any{"query": args.Query},
		})
		if bsErr == nil {
			meta := map[string]any{
				"query":           args.Query,
				"source":          "browser_search_fallback",
				"http_chain_dead": true,
			}
			if bsResult.Metadata != nil {
				if engine, ok := bsResult.Metadata["engine"]; ok {
					meta["engine"] = engine
				}
				if u, ok := bsResult.Metadata["url"]; ok {
					meta["url"] = u
				}
			}
			return Result{
				Output:   bsResult.Output,
				Metadata: meta,
			}, nil
		}
		// browser_search juga gagal — kasih error gabungan.
		msg := "Web search HTTP endpoints semua dead (DDG/SearXNG/Brave) + browser_search fallback ALSO failed."
		if lastErr != nil {
			msg += " HTTP last error: " + lastErr.Error() + "."
		}
		msg += " Browser fallback error: " + bsErr.Error()
		return Result{
			Output:   msg,
			Metadata: map[string]any{"query": args.Query, "count": 0, "error": "all_search_paths_dead"},
		}, nil
	}

	// SearXNG JSON response handled separately.
	if strings.Contains(winningEndpoint, "searx") && strings.Contains(winningEndpoint, "format=json") {
		return parseSearxJSON(body, args.Query, limit)
	}

	matches := ddgResultRe.FindAllStringSubmatch(string(body), limit)
	if len(matches) == 0 {
		// 2026-05-05 update: kalau parser DDG ngga match (mis. Brave HTML
		// struktur beda) → coba parseGenericHTML. Kalau hasilnya nav junk
		// (>50% link ke search-engine sendiri / tombol Premium dll) →
		// AUTO-DELEGATE ke browser_search. Sebelumnya cuma prepend warning
		// + return junk → warga halu lapor "context deadline exceeded" /
		// "search ngga jalan" ke keluh-kesah. Sekarang sukses dengan path
		// alternatif transparently.
		generic := parseGenericHTML(body, args.Query, limit, winningEndpoint)
		if !isNavJunk(generic, winningEndpoint) {
			return generic, nil
		}
		bs := NewBrowserSearchTool()
		bsArgs, _ := json.Marshal(map[string]any{"query": args.Query})
		bsResult, bsErr := bs.Execute(ctx, Invocation{
			ToolName:   "browser_search",
			Arguments:  bsArgs,
			ParsedArgs: map[string]any{"query": args.Query},
		})
		if bsErr == nil {
			meta := map[string]any{
				"query":            args.Query,
				"source":           "browser_search_fallback",
				"http_engine":      winningEndpoint,
				"http_engine_junk": true,
			}
			if bsResult.Metadata != nil {
				if engine, ok := bsResult.Metadata["engine"]; ok {
					meta["engine"] = engine
				}
				if u, ok := bsResult.Metadata["url"]; ok {
					meta["url"] = u
				}
			}
			return Result{
				Output:   bsResult.Output,
				Metadata: meta,
			}, nil
		}
		// browser_search gagal — return generic dengan warning sebagai last resort.
		generic.Output = "[websearch HTTP scraping degraded + browser_search fallback failed: " +
			bsErr.Error() + "]\n\n" + generic.Output
		return generic, nil
	}

	var sb strings.Builder
	results := make([]map[string]string, 0, len(matches))
	for i, m := range matches {
		if i >= limit {
			break
		}
		linkURL := decodeDDGLink(m[1])
		title := stripHTML(m[2])
		snippet := stripHTML(m[3])
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, title, linkURL, snippet)
		results = append(results, map[string]string{
			"title":   title,
			"url":     linkURL,
			"snippet": snippet,
		})
	}

	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"query":   args.Query,
			"count":   len(results),
			"results": results,
		},
	}, nil
}

// decodeDDGLink — DuckDuckGo wraps result URLs in `/l/?uddg=ENCODED`. Unwrap.
func decodeDDGLink(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Path == "/l/" {
		if real := u.Query().Get("uddg"); real != "" {
			if decoded, err := url.QueryUnescape(real); err == nil {
				return decoded
			}
		}
	}
	return raw
}

func stripHTML(s string) string {
	s = htmlStripper.ReplaceAllString(s, "")
	s = htmlEntityRe.ReplaceAllStringFunc(s, func(e string) string {
		switch e {
		case "&amp;":
			return "&"
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		case "&quot;":
			return `"`
		case "&#39;":
			return "'"
		default:
			// no-op — exhaustive switch guard
		}
		return ""
	})
	s = spaceStripper.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// isNavJunk reports whether a generic-HTML parse returned mostly navigation
// links (e.g. Brave's "Premium", "Transparency Report") from the search
// engine's own domain instead of real external results. Trigger for the
// OpenRouter :online fallback.
func isNavJunk(r Result, sourceEndpoint string) bool {
	meta, ok := r.Metadata["results"].([]map[string]string)
	if !ok || len(meta) == 0 {
		return true
	}
	// Extract the search-engine host (e.g. "search.brave.com") to flag URLs
	// that point back to it.
	engineHost := ""
	if u, err := url.Parse(sourceEndpoint); err == nil {
		engineHost = strings.TrimPrefix(u.Host, "www.")
		// Collapse subdomain to base domain for brave (search.brave.com -> brave.com)
		if parts := strings.Split(engineHost, "."); len(parts) >= 2 {
			engineHost = parts[len(parts)-2] + "." + parts[len(parts)-1]
		}
	}
	junkCount := 0
	for _, res := range meta {
		link := res["url"]
		if strings.Contains(link, engineHost) {
			junkCount++
			continue
		}
		// Common nav patterns: "premium", "transparency", "about", "signup", "login"
		lowerTitle := strings.ToLower(res["title"])
		for _, p := range []string{"premium", "transparency", "sign up", "log in", "settings", "preferences"} {
			if strings.Contains(lowerTitle, p) {
				junkCount++
				break
			}
		}
	}
	return junkCount*2 > len(meta) // >50% junk
}

// parseSearxJSON extracts results from a SearXNG JSON response.
func parseSearxJSON(body []byte, query string, limit int) (Result, error) {
	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results" validate:"required"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Result{
			Output:   fmt.Sprintf("Searx returned unparseable JSON for: %s", query),
			Metadata: map[string]any{"query": query, "count": 0},
		}, nil
	}
	if len(parsed.Results) == 0 {
		return Result{
			Output:   fmt.Sprintf("No results found for: %s", query),
			Metadata: map[string]any{"query": query, "count": 0, "source": "searx"},
		}, nil
	}
	var sb strings.Builder
	out := make([]map[string]string, 0, limit)
	for i, r := range parsed.Results {
		if i >= limit {
			break
		}
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Content)
		out = append(out, map[string]string{
			"title": r.Title, "url": r.URL, "snippet": r.Content,
		})
	}
	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"query": query, "count": len(out), "results": out, "source": "searx",
		},
	}, nil
}

// parseGenericHTML is a last-ditch parser that extracts any <a href> links
// with their surrounding text. Good enough to give the model *something*
// when DDG/Searx patterns don't match (Brave, Google fallback, etc).
var genericLinkRe = regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]*>([^<]{5,200})</a>`)

func parseGenericHTML(body []byte, query string, limit int, source string) Result {
	matches := genericLinkRe.FindAllStringSubmatch(string(body), limit*4) // grab extra, filter noise
	if len(matches) == 0 {
		return Result{
			Output:   fmt.Sprintf("No results parseable from %s for: %s", source, query),
			Metadata: map[string]any{"query": query, "count": 0},
		}
	}
	var sb strings.Builder
	out := make([]map[string]string, 0, limit)
	seen := map[string]bool{}
	for _, m := range matches {
		if len(out) >= limit {
			break
		}
		link := m[1]
		text := stripHTML(m[2])
		if seen[link] || len(text) < 10 {
			continue
		}
		// Skip navigation / static asset links.
		if strings.Contains(link, "google.com/search") || strings.Contains(link, "webcache") ||
			strings.Contains(link, "accounts.google.com") || strings.Contains(link, "/preferences") {
			continue
		}
		seen[link] = true
		fmt.Fprintf(&sb, "%d. %s\n   %s\n\n", len(out)+1, text, link)
		out = append(out, map[string]string{"title": text, "url": link})
	}
	if len(out) == 0 {
		return Result{
			Output:   fmt.Sprintf("Results ditemukan tapi tidak bisa diekstrak cleanly (%s). Query: %s", source, query),
			Metadata: map[string]any{"query": query, "count": 0, "source": source},
		}
	}
	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"query": query, "count": len(out), "results": out, "source": source,
		},
	}
}
