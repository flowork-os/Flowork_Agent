// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev). Locked: 2026-06-02.
// Reason: FASE 7 MCP server. E2E verified (initialize/tools/list/tools/call →
//
//	task_list+task_run+task_result drive Flowork beneran). Extend tool → tambah
//	di toolDefs + callTool.
//	2026-06-06: +tool `chat` (Connections — MCP jadi connector first-class: chat ke
//	agent via /api/kernel/rpc, JALUR SAMA Telegram/CLI). Owner-authorized extend.
//
// flowork-mcp — FASE 7: MCP (Model Context Protocol) server buat AI EKSTERNAL
// (Claude Desktop/Code, Cursor, dll) drive Flowork. 1-pintu: tool MCP →
// endpoint lokal → JALUR SAMA kayak chat/Telegram (doktrin funnel).
//
// Transport: stdio, JSON-RPC 2.0 newline-delimited (MCP stdio standard).
// Tools yang di-expose: chat (connector ke agent), task_list, task_run, task_result.
// Connector self-config: agent tujuan dari env FLOWORK_MCP_AGENT atau
// ~/.flowork/connectors/mcp/config.json (key "agent"/"TARGET_AGENT"), default mr-flow-next.
//
// Wiring (mcp.json AI eksternal):
//
//	{ "mcpServers": { "flowork": { "command": "/path/to/flowork-mcp" } } }
//
// Env: FLOWORK_SELF_URL (default http://127.0.0.1:1987) — server Flowork.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "flowork"
	serverVersion   = "1.0.0"
)

func selfURL() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_SELF_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:1987"
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// ── JSON-RPC types ───────────────────────────────────────────────────────────

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // absent = notification
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
			continue // ga bisa parse → skip (ga ada id buat balas)
		}
		resp, isNotif := handle(req)
		if isNotif {
			continue // notification → ga ada balasan
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
		return resp, true // notification
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs()}
	case "tools/call":
		resp.Result = callTool(req.Params)
	default:
		if len(req.ID) == 0 {
			return resp, true // unknown notification
		}
		resp.Error = &rpcErr{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp, false
}

// ── Tool definitions (MCP shape) ─────────────────────────────────────────────

func toolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "chat",
			"description": "Chat with a Flowork agent (default mr-flow) — the SAME brain Telegram and the CLI talk to. Send a message, get the agent's natural-language reply (it can use its tools, memory, etc).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string", "description": "what to say to the agent"},
					"agent":   map[string]any{"type": "string", "description": "agent id to talk to (optional, default mr-flow)"},
				},
				"required": []string{"message"},
			},
		},
		{
			"name":        "task_list",
			"description": "Daftar Category Task (analisa multi-agent) yang tersedia di Flowork.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "task_run",
			"description": "Trigger Category Task di Flowork (crew analis → 1 keputusan). ASYNC: balik run_id; cek hasil pakai task_result.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{"type": "string", "description": "id kategori (dari task_list, mis. 'saham')"},
					"subject":  map[string]any{"type": "string", "description": "subjek analisa (mis. 'BBCA')"},
				},
				"required": []string{"category", "subject"},
			},
		},
		{
			"name":        "task_result",
			"description": "Ambil status + hasil 1 run task (timeline per-agent + keputusan kalau udah selesai).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{"type": "integer", "description": "run_id dari task_run"},
				},
				"required": []string{"run_id"},
			},
		},
	}
}

// ── tools/call dispatch ──────────────────────────────────────────────────────

func callTool(params json.RawMessage) map[string]any {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	_ = json.Unmarshal(params, &p)
	var text string
	var err error
	switch p.Name {
	case "chat":
		msg, _ := p.Arguments["message"].(string)
		agent, _ := p.Arguments["agent"].(string)
		text, err = callChat(msg, agent)
	case "task_list":
		text, err = httpText("GET", "/api/taskflow/categories", nil)
	case "task_run":
		cat, _ := p.Arguments["category"].(string)
		subj, _ := p.Arguments["subject"].(string)
		q := url.Values{"category": {cat}, "subject": {subj}}
		text, err = httpText("POST", "/api/taskflow/run?"+q.Encode(), nil)
	case "task_result":
		rid := fmt.Sprintf("%v", p.Arguments["run_id"])
		text, err = httpText("GET", "/api/taskflow/run-detail?id="+url.QueryEscape(rid), nil)
	default:
		err = fmt.Errorf("unknown tool: %s", p.Name)
	}
	if err != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": "ERROR: " + err.Error()}},
			"isError": true,
		}
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
}

// callChat bridges an MCP chat tool-call to a Flowork agent via the loopback kernel
// RPC (handle_message) — the SAME message path Telegram and the CLI connector drive,
// so the reply is identical. Empty agent → the connector's configured default.
func callChat(message, agent string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("message required")
	}
	if strings.TrimSpace(agent) == "" {
		agent = mcpAgent()
	}
	body, _ := json.Marshal(map[string]any{
		"plugin":   agent,
		"function": "handle_message",
		"args":     map[string]any{"text": message},
	})
	raw, err := httpText("POST", "/api/kernel/rpc", body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Reply string `json:"reply"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(raw), &parsed)
	if parsed.Error != "" {
		return "", fmt.Errorf("%s", parsed.Error)
	}
	if parsed.Reply != "" {
		return parsed.Reply, nil
	}
	return raw, nil // unknown shape — return raw so nothing is hidden
}

// mcpAgent resolves which agent this MCP connector chats with. Self-managed config:
// env FLOWORK_MCP_AGENT, else ~/.flowork/connectors/mcp/config.json (key "agent" or
// "TARGET_AGENT"), else mr-flow-next. Paths via filepath → multi-OS.
func mcpAgent() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_MCP_AGENT")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if raw, rerr := os.ReadFile(filepath.Join(home, ".flowork", "connectors", "mcp", "config.json")); rerr == nil {
			var c map[string]string
			if json.Unmarshal(raw, &c) == nil {
				if c["agent"] != "" {
					return c["agent"]
				}
				if c["TARGET_AGENT"] != "" {
					return c["TARGET_AGENT"]
				}
			}
		}
	}
	return defaultMCPAgent() // default mr-flow (ENV FLOWORK_ORCHESTRATOR override); mr-flow-next belum ke-deploy — lock/mrflow.md §6b
}

// defaultMCPAgent — fallback agent target. SATU switch dgn host: ENV FLOWORK_ORCHESTRATOR,
// default mr-flow (orchestrator LIVE).
func defaultMCPAgent() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ORCHESTRATOR")); v != "" {
		return v
	}
	return "mr-flow"
}

// httpText — call endpoint Flowork lokal, balikin body sebagai text.
func httpText(method, path string, body []byte) (string, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, selfURL()+path, r)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	return string(b), nil
}
