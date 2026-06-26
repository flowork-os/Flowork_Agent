// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

type Config struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type Server struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner

	mu     sync.Mutex
	nextID int
	closed bool
}

func Start(ctx context.Context, name string, cfg Config) (*Server, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("mcp[%s]: command required", name)
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
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
	cmd.Stderr = io.Discard
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

func (s *Server) Name() string { return s.name }

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
		return string(raw), nil
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
	_ = s.cmd.Wait()
	return nil
}

type rpcResp struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

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
				continue
			}
			var gotID int
			if json.Unmarshal(r.ID, &gotID) != nil || gotID != id {
				continue
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

		s.closed = true
		_ = s.stdin.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		return nil, ctx.Err()
	}
}

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
