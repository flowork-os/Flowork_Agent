// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: MCP client transport (ROADMAP_MCP_CONNECTORS.md Phase 1). stdio JSON-RPC to
//
//	an external MCP server — start/initialize/tools.list/tools.call/close. The
//	serialized request/response + timeout-kills-process logic is concurrency-critical
//	(race-tested); don't weaken it. Dogfood-verified against bin/flowork-mcp.
//
// Package mcpclient is the Flowork side of an MCP CLIENT: it spawns an external MCP
// server (the standard stdio transport — a command + args + env, exactly the shape
// you paste into Claude Desktop's mcpServers) and speaks JSON-RPC 2.0 to it over
// stdin/stdout.
//
// This is "Jenis 2" connector plumbing (ROADMAP_MCP_CONNECTORS.md): an MCP server
// (github / filesystem / …) is a TOOL-SOURCE. The registry layer above lists the
// server's tools and registers each into Flowork's tool registry, so agents reach
// them through the tool system they already use (tool_search → tool.run). This file
// only owns the transport: start · initialize · tools/list · tools/call · close.
//
// Multi-OS: pure-Go os/exec + pipes — no platform-specific calls.
package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

const protocolVersion = "2024-11-05"

// Config is an MCP server's launch spec — the same fields as a Claude Desktop
// mcpServers entry.
type Config struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Tool is one tool a server advertises.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Server is a running MCP server connection. Safe for concurrent CallTool — each
// call is serialized (one JSON-RPC request/response at a time over the pipe).
type Server struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner

	mu     sync.Mutex
	nextID int
	closed bool
}

// Start spawns the MCP server and completes the initialize handshake. The returned
// Server keeps the process alive until Close. ctx bounds only the handshake.
func Start(ctx context.Context, name string, cfg Config) (*Server, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("mcp[%s]: command required", name)
	}
	cmd := exec.Command(cfg.Command, cfg.Args...) // lifecycle managed via Close, not ctx
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp[%s] stdin: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp[%s] stdout: %w", name, err)
	}
	cmd.Stderr = io.Discard // server logs to stderr; we don't relay them
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp[%s] start: %w", name, err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	s := &Server{name: name, cmd: cmd, stdin: stdin, scanner: sc}

	if _, err := s.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "flowork", "version": "1.0.0"},
	}); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("mcp[%s] initialize: %w", name, err)
	}
	_ = s.notify("notifications/initialized", nil)
	return s, nil
}

// Name returns the connector name this server belongs to.
func (s *Server) Name() string { return s.name }

// ListTools returns the tools the server advertises (tools/list).
func (s *Server) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := s.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp[%s] tools/list parse: %w", s.name, err)
	}
	return out.Tools, nil
}

// CallTool invokes one tool (tools/call) and returns its result text. MCP returns
// {content:[{type:"text",text:...}], isError}; we join the text parts.
func (s *Server) CallTool(ctx context.Context, tool string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := s.call(ctx, "tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw), nil // unknown shape — return raw so nothing is hidden
	}
	text := ""
	for _, c := range out.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	if out.IsError {
		return "", fmt.Errorf("mcp[%s] %s: %s", s.name, tool, text)
	}
	return text, nil
}

// Close terminates the server process.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait() // reaps the process; returns "signal: killed" which is expected
	return nil
}

// rpcResp is the subset of a JSON-RPC response we read.
type rpcResp struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call sends one JSON-RPC request and reads until the matching id (skipping
// notifications). Serialized by mu so the shared pipe carries one exchange at a time.
// ctx cancels the wait (the read goroutine unblocks when the process is later killed).
func (s *Server) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("mcp[%s]: closed", s.name)
	}
	s.nextID++
	id := s.nextID
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, _ := json.Marshal(req)
	if _, err := s.stdin.Write(append(b, '\n')); err != nil {
		return nil, fmt.Errorf("mcp[%s] write: %w", s.name, err)
	}

	type readOut struct {
		res json.RawMessage
		err error
	}
	ch := make(chan readOut, 1)
	go func() {
		for s.scanner.Scan() {
			var r rpcResp
			if json.Unmarshal(s.scanner.Bytes(), &r) != nil || len(r.ID) == 0 {
				continue // unparseable or a notification — skip
			}
			var gotID int
			if json.Unmarshal(r.ID, &gotID) != nil || gotID != id {
				continue // a different request's response — skip
			}
			if r.Error != nil {
				ch <- readOut{err: fmt.Errorf("mcp[%s] %s: %s", s.name, method, r.Error.Message)}
				return
			}
			ch <- readOut{res: r.Result}
			return
		}
		if err := s.scanner.Err(); err != nil {
			ch <- readOut{err: err}
			return
		}
		ch <- readOut{err: fmt.Errorf("mcp[%s] %s: server closed", s.name, method)}
	}()

	select {
	case out := <-ch:
		return out.res, out.err
	case <-ctx.Done():
		// The server didn't answer in time. Kill it so the read goroutine's Scan
		// returns (it then sends to the buffered ch and exits — no leak) and mark the
		// server closed, so a NEXT call can't start a second reader on the same pipe
		// (which would race the orphaned one). A timed-out MCP server is dead anyway.
		s.closed = true
		_ = s.stdin.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		return nil, ctx.Err()
	}
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (s *Server) notify(method string, params any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("mcp[%s]: closed", s.name)
	}
	req := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		req["params"] = params
	}
	b, _ := json.Marshal(req)
	_, err := s.stdin.Write(append(b, '\n'))
	return err
}
