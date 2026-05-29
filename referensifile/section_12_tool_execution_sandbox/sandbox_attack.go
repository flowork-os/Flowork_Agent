// Package tools — sandbox_attack.go
//
// Phase 2 Sandbox Practice tool (per phase-progression-doctrine.md). Auto-armed
// saat DEBATE_PHASE2_UNLOCK=true (≥500 quality debate samples).
//
// Per Ayah arahan 2026-05-10: warga harus practice hands-on, theory aja ngga
// cukup. Tool ini executor untuk payload yang warga propose terhadap sandbox
// target (DVWA, Juice Shop) — return real HTTP response sehingga warga dapat
// feedback yang grounded (anti-paper-hacker).
//
// CAPABILITY GATE STRICT:
//   - target_url HARUS match prefix dari settings DB SANDBOX_*_URL keys
//   - SANDBOX_PRACTICE_ENABLED HARUS 'true'
//   - Reject ANY URL yang bukan localhost sandbox (anti-misuse: warga ngga
//     boleh attack real target dari sini, itu jalur Phase 4 via oss-hunter)

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
)

type SandboxAttackTool struct {
	client *http.Client
	// settingsReader injected (workspace-aware to read SANDBOX_*_URL + enable flag)
	settingsReader func(key string) string
}

type sandboxAttackArgs struct {
	TargetURL    string            `json:"target_url" validate:"required"`
	VulnCategory string            `json:"vuln_category" validate:"required"` // sqli|xss|csrf|rce|upload|idor|ssrf|xxe
	Payload      string            `json:"payload" validate:"required"`
	Method       string            `json:"method,omitempty"`     // GET|POST (default GET)
	Headers      map[string]string `json:"headers,omitempty"`    // optional custom headers
	BodyFields   map[string]string `json:"body_fields,omitempty"` // POST form fields (URL-encoded)
	Cookie       string            `json:"cookie,omitempty"`      // Cookie header value (e.g. session)
	Technique    string            `json:"technique,omitempty"`   // human-readable technique tag
}

func NewSandboxAttackTool(settingsReader func(key string) string) *SandboxAttackTool {
	// Plain http.Client (NOT safeclient) — sandbox_attack TUJUANNYA attack localhost.
	// Safeclient SSRF protection (block private/loopback IP) akan reject DVWA target.
	// Capability gate di Execute() kita sendiri yang restrict ke SANDBOX_*_URL allowed list.
	return &SandboxAttackTool{
		client:         &http.Client{Timeout: 15 * time.Second},
		settingsReader: settingsReader,
	}
}

func (t *SandboxAttackTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "sandbox_attack",
		Description: "Execute security payload terhadap sandbox practice target (DVWA / Juice Shop). " +
			"CAPABILITY-GATED: hanya allow URL prefix dari settings DB SANDBOX_*_URL. " +
			"Returns HTTP response untuk warga reasoning + scoring exploit success.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target_url": map[string]any{
					"type":        "string",
					"description": "Target URL (HARUS prefix dengan SANDBOX_*_URL dari settings DB).",
				},
				"vuln_category": map[string]any{
					"type":        "string",
					"description": "Kategori vuln: sqli, xss, csrf, rce, upload, idor, ssrf, xxe.",
				},
				"payload": map[string]any{
					"type":        "string",
					"description": "Exploit payload string (e.g. \"' OR 1=1 --\" untuk SQLi).",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method: GET atau POST (default GET).",
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "Optional custom headers (key-value).",
				},
				"body_fields": map[string]any{
					"type":        "object",
					"description": "POST form fields (key-value, URL-encoded).",
				},
				"cookie": map[string]any{
					"type":        "string",
					"description": "Cookie header value (untuk session-based exploit).",
				},
				"technique": map[string]any{
					"type":        "string",
					"description": "Technique tag (e.g. 'union-based-sqli', 'reflected-xss-img-tag').",
				},
			},
			"required": []string{"target_url", "vuln_category", "payload"},
		},
	}
}

func (t *SandboxAttackTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args sandboxAttackArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode sandbox_attack arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("validation failed: %w", err)
	}

	// Capability gate 1: feature enabled?
	if t.settingsReader == nil {
		return Result{
			Output:   "ERR sandbox_attack: settings reader not injected (deployment misconfig).",
			Metadata: map[string]any{"error": "settings_reader_nil"},
		}, nil
	}
	enabled := strings.ToLower(strings.TrimSpace(t.settingsReader("SANDBOX_PRACTICE_ENABLED")))
	if enabled != "true" {
		return Result{
			Output:   "ERR sandbox_attack DISABLED: SANDBOX_PRACTICE_ENABLED != true. Phase 2 belum unlock atau bootstrap belum jalan.",
			Metadata: map[string]any{"error": "feature_disabled"},
		}, nil
	}

	// Capability gate 2: target URL must match allowed sandbox prefix
	allowedPrefixes := []string{}
	for _, key := range []string{"SANDBOX_DVWA_URL", "SANDBOX_JUICESHOP_URL", "SANDBOX_VULHUB_URL"} {
		if u := strings.TrimSpace(t.settingsReader(key)); u != "" {
			allowedPrefixes = append(allowedPrefixes, strings.TrimRight(u, "/"))
		}
	}
	if len(allowedPrefixes) == 0 {
		return Result{
			Output:   "ERR sandbox_attack: NO sandbox URL configured. Run scripts/bootstrap_phase2.sh first.",
			Metadata: map[string]any{"error": "no_sandbox_configured"},
		}, nil
	}
	matched := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(args.TargetURL, prefix) {
			matched = true
			break
		}
	}
	if !matched {
		return Result{
			Output: fmt.Sprintf("ERR sandbox_attack REJECTED: target_url=%s not in allowed prefixes=%v. "+
				"Capability gate strict: hanya sandbox localhost URL yang allowed (anti-misuse, Phase 4 real target via oss-hunter).",
				args.TargetURL, allowedPrefixes),
			Metadata: map[string]any{"error": "target_url_not_allowed", "allowed_prefixes": allowedPrefixes},
		}, nil
	}

	// Validate vuln_category whitelist
	allowedCategories := map[string]bool{
		"sqli": true, "xss": true, "csrf": true, "rce": true,
		"upload": true, "idor": true, "ssrf": true, "xxe": true,
		"path-traversal": true, "auth-bypass": true, "open-redirect": true,
	}
	if !allowedCategories[strings.ToLower(args.VulnCategory)] {
		return Result{
			Output:   fmt.Sprintf("ERR sandbox_attack: vuln_category '%s' not in whitelist (sqli/xss/csrf/rce/upload/idor/ssrf/xxe/path-traversal/auth-bypass/open-redirect)", args.VulnCategory),
			Metadata: map[string]any{"error": "vuln_category_invalid"},
		}, nil
	}

	// Build HTTP request
	method := strings.ToUpper(strings.TrimSpace(args.Method))
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "POST" {
		return Result{
			Output:   fmt.Sprintf("ERR sandbox_attack: method '%s' not supported (only GET/POST)", method),
			Metadata: map[string]any{"error": "method_invalid"},
		}, nil
	}

	var bodyReader io.Reader
	contentType := ""
	if method == "POST" && len(args.BodyFields) > 0 {
		formParts := []string{}
		for k, v := range args.BodyFields {
			formParts = append(formParts, fmt.Sprintf("%s=%s", k, v))
		}
		formBody := strings.Join(formParts, "&")
		bodyReader = strings.NewReader(formBody)
		contentType = "application/x-www-form-urlencoded"
	}

	req, err := http.NewRequestWithContext(ctx, method, args.TargetURL, bodyReader)
	if err != nil {
		return Result{
			Output:   fmt.Sprintf("ERR sandbox_attack: build request failed: %v", err),
			Metadata: map[string]any{"error": "build_request_failed"},
		}, nil
	}

	// Standard headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Flowork Sandbox Practice; +localhost-only)")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if args.Cookie != "" {
		req.Header.Set("Cookie", args.Cookie)
	}
	for k, v := range args.Headers {
		req.Header.Set(k, v)
	}

	// Execute
	t0 := time.Now()
	resp, err := t.client.Do(req)
	latency := time.Since(t0)
	if err != nil {
		return Result{
			Output: fmt.Sprintf("sandbox_attack request failed: %v (latency %v). "+
				"Check sandbox container is up.", err, latency),
			Metadata: map[string]any{"error": "request_failed", "latency_ms": latency.Milliseconds()},
		}, nil
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // cap 64KB
	bodyStr := string(bodyBytes)

	// Heuristic: detect exploit landing signal
	exploitLanded, evidence := detectExploitLanding(args.VulnCategory, args.Payload, resp.StatusCode, bodyStr)

	// Compose output text
	output := fmt.Sprintf(`=== SANDBOX ATTACK RESULT ===
target_url: %s
vuln_category: %s
method: %s
payload: %s
technique: %s
http_status: %d
latency_ms: %d
body_length: %d
exploit_landed: %v
evidence: %s

=== RESPONSE BODY (first 2KB) ===
%s
`, args.TargetURL, args.VulnCategory, method, args.Payload, args.Technique,
		resp.StatusCode, latency.Milliseconds(), len(bodyBytes), exploitLanded, evidence,
		bodyStr[:min(2048, len(bodyStr))])

	return Result{
		Output: output,
		Metadata: map[string]any{
			"target_url":      args.TargetURL,
			"vuln_category":   args.VulnCategory,
			"payload":         args.Payload,
			"method":          method,
			"http_status":     resp.StatusCode,
			"latency_ms":      latency.Milliseconds(),
			"body_length":     len(bodyBytes),
			"exploit_landed":  exploitLanded,
			"evidence":        evidence,
			"response_snippet": bodyStr[:min(1000, len(bodyStr))],
		},
	}, nil
}

// detectExploitLanding heuristic per vuln category. Returns (landed, evidence).
// Heuristics intentionally simple — score warga via separate judge LLM call,
// this only provides ground-truth feedback signal.
func detectExploitLanding(category, payload string, status int, body string) (bool, string) {
	bodyLower := strings.ToLower(body)

	switch strings.ToLower(category) {
	case "sqli":
		// Common SQLi success indicators: SQL error message leakage, OR-1=1 returning all records,
		// UNION SELECT data extraction
		sqlErrorPatterns := []string{
			"sql syntax", "mysql_fetch", "mysqli_fetch", "you have an error in your sql",
			"warning: mysql", "ora-", "postgresql", "syntax error at or near",
			"unclosed quotation mark", "microsoft ole db provider",
			"odbc microsoft", "supplied argument is not a valid mysql",
		}
		for _, p := range sqlErrorPatterns {
			if strings.Contains(bodyLower, p) {
				return true, "SQL error leakage: " + p
			}
		}
		// OR 1=1 may dump multiple rows — check for row indicators
		if strings.Contains(payload, "1=1") || strings.Contains(payload, "or 1=") {
			rowCount := strings.Count(bodyLower, "<tr") - strings.Count(bodyLower, "</tr")
			if rowCount > 5 {
				return true, fmt.Sprintf("multi-row leak detected (%d rows)", rowCount)
			}
		}

	case "xss":
		// XSS landed = payload appears unescaped in body
		xssMarkers := []string{"<script>", "<img ", "<svg ", "onerror=", "onload="}
		for _, m := range xssMarkers {
			if strings.Contains(payload, m) && strings.Contains(body, m) {
				return true, fmt.Sprintf("payload reflected unescaped: %s", m)
			}
		}

	case "rce", "command-injection":
		// RCE indicators: command output leaked (uname, ls, whoami signatures)
		rcePatterns := []string{
			"uid=", "gid=", "groups=",
			"linux ", "darwin ", "/etc/passwd", "root:x:",
			"bin:x:", "daemon:x:", "windows nt",
		}
		for _, p := range rcePatterns {
			if strings.Contains(bodyLower, p) {
				return true, "command output leak: " + p
			}
		}

	case "path-traversal", "lfi":
		// LFI/path-traversal: /etc/passwd contents, win.ini contents
		if strings.Contains(bodyLower, "root:x:") || strings.Contains(bodyLower, "[fonts]") {
			return true, "system file content leaked"
		}

	case "ssrf":
		// SSRF: aws metadata, internal-only response patterns
		ssrfMarkers := []string{
			"169.254.169.254", "iam credentials", "ami-id", "instance-id",
			"<?xml", "internal server",
		}
		for _, m := range ssrfMarkers {
			if strings.Contains(bodyLower, m) {
				return true, "SSRF response signature: " + m
			}
		}

	case "auth-bypass", "idor":
		// 200 OK on protected endpoint without proper auth
		if status == 200 && (strings.Contains(bodyLower, "admin") || strings.Contains(bodyLower, "dashboard")) {
			return true, fmt.Sprintf("200 OK with admin/dashboard markers (auth bypass likely)")
		}

	case "upload":
		// File upload success indicator
		if strings.Contains(bodyLower, "uploaded") || strings.Contains(bodyLower, "succesful") {
			return true, "upload success message detected"
		}
	}

	// Default: no clear landing signal
	if status == 200 {
		return false, fmt.Sprintf("HTTP 200 but no exploit signature for category=%s", category)
	}
	return false, fmt.Sprintf("HTTP %d, no exploit signature", status)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
