// flowork-mcp-web — a tiny, sovereign (Go, no npm) MCP server that gives Flowork
// agents READ-ONLY public web data the engine doesn't expose natively. Transport:
// stdio JSON-RPC 2.0, newline-delimited (MCP stdio standard) — same shape as
// cmd/flowork-mcp. Install it via Connections → MCP; each tool then registers as
// mcp_<id>_<tool> and any agent can call it through the loket tool.run.
//
// Tools:
//   - github_repo: real metadata for a public repo (stars, language, topics,
//     description, open issues, last push, license) — used to ground honest reviews.
//
// Env: GITHUB_TOKEN (optional) — lifts the GitHub API rate limit from 60 to 5000/h.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "flowork-web"
	serverVersion   = "1.0.0"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req rpcReq
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		resp, isNotif := handle(req)
		if isNotif {
			continue
		}
		b, _ := json.Marshal(resp)
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}

func handle(req rpcReq) (rpcResp, bool) {
	resp := rpcResp{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
		}
	case "notifications/initialized", "initialized":
		return resp, true
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs()}
	case "tools/call":
		resp.Result = callTool(req.Params)
	default:
		if len(req.ID) == 0 {
			return resp, true
		}
		resp.Error = &rpcErr{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp, false
}

func toolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "github_repo",
			"description": "Real metadata for a public GitHub repo: stars, primary language, topics, description, open issues, last push date, license. Use it to ground an honest review with real numbers.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repo": map[string]any{"type": "string", "description": "owner/repo, e.g. apple/container"},
				},
				"required": []string{"repo"},
			},
		},
	}
}

func textResult(s string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": s}}}
}

func errResult(s string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": s}}, "isError": true}
}

func callTool(params json.RawMessage) map[string]any {
	var p struct {
		Name string `json:"name"`
		Args struct {
			Repo string `json:"repo"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errResult("bad params: " + err.Error())
	}
	switch p.Name {
	case "github_repo":
		return githubRepo(strings.TrimSpace(p.Args.Repo))
	default:
		return errResult("unknown tool: " + p.Name)
	}
}

func githubRepo(slug string) map[string]any {
	if slug == "" || !strings.Contains(slug, "/") {
		return errResult("repo must be owner/repo")
	}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+slug, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "flowork-mcp-web")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return errResult("fetch: " + err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errResult(fmt.Sprintf("github status %d", resp.StatusCode))
	}
	var r struct {
		FullName    string   `json:"full_name"`
		Description string   `json:"description"`
		Stars       int      `json:"stargazers_count"`
		Forks       int      `json:"forks_count"`
		OpenIssues  int      `json:"open_issues_count"`
		Language    string   `json:"language"`
		Topics      []string `json:"topics"`
		PushedAt    string   `json:"pushed_at"`
		CreatedAt   string   `json:"created_at"`
		Homepage    string   `json:"homepage"`
		Archived    bool     `json:"archived"`
		License     struct {
			Name string `json:"name"`
		} `json:"license"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return errResult("decode: " + err.Error())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", r.FullName, r.Description)
	fmt.Fprintf(&b, "stars: %d | forks: %d | open issues: %d\n", r.Stars, r.Forks, r.OpenIssues)
	if r.Language != "" {
		fmt.Fprintf(&b, "language: %s\n", r.Language)
	}
	if len(r.Topics) > 0 {
		fmt.Fprintf(&b, "topics: %s\n", strings.Join(r.Topics, ", "))
	}
	if r.License.Name != "" {
		fmt.Fprintf(&b, "license: %s\n", r.License.Name)
	}
	if len(r.PushedAt) >= 10 {
		fmt.Fprintf(&b, "last push: %s | created: %s\n", r.PushedAt[:10], safeDate(r.CreatedAt))
	}
	if r.Archived {
		b.WriteString("⚠️ archived (no longer maintained)\n")
	}
	if r.Homepage != "" {
		fmt.Fprintf(&b, "homepage: %s\n", r.Homepage)
	}
	return textResult(strings.TrimSpace(b.String()))
}

func safeDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
