// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: MCP Servers → dok lock/gui/MCP Servers.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/mcpsecurity"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func mcpRouterHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/mcp")
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		mcpListUpsertHandler(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if len(parts) == 2 {
		switch parts[1] {
		case "tools":
			mcpToolsHandler(w, r, id)
			return
		case "message":
			mcpGatewayMessageHandler(w, r, id)
			return
		case "sse":
			mcpGatewaySSEHandler(w, r, id)
			return
		}
	}
	mcpCRUDHandler(w, r, id)
}

func mcpGatewayMessageHandler(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	srv, err := store.GetMCPServer(d, id)
	if err != nil || srv == nil {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	msg, _ := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch srv.Transport {
	case "http", "sse":
		endpoint := srv.URL
		if _, verr := blockMetadataURL(ctx, endpoint); verr != nil {
			http.Error(w, "blocked target (SSRF guard): "+verr.Error(), http.StatusForbidden)
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(msg))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := mediaHTTPClient.Do(req)
		if err != nil {
			http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	default:
		out, err := mcpStdioRoundTrip(ctx, srv, msg)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out)
	}
}

func mcpStdioRoundTrip(ctx context.Context, srv *store.MCPServer, msg []byte) ([]byte, error) {
	if srv.Command == "" {
		return nil, fmt.Errorf("stdio server missing command")
	}

	if !mcpsecurity.IsAllowed(srv.Command) {
		return nil, fmt.Errorf("command %q is not on the MCP allowlist (npx/node/uvx/python/bunx/bun/deno/pnpm/yarn). Run mcpsecurity.Allow(%q) to extend.", srv.Command, filepath.Base(srv.Command))
	}
	if _, err := exec.LookPath(srv.Command); err != nil {
		return nil, fmt.Errorf("command %q not found", srv.Command)
	}
	var wantID any
	var probe map[string]any
	if json.Unmarshal(msg, &probe) == nil {
		wantID = probe["id"]
	}
	cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn: %w", err)
	}

	defer stdin.Close()
	defer stdout.Close()
	defer func() { _ = cmd.Process.Kill() }()
	_, _ = stdin.Write(jsonRPCMsg(1, "initialize", mcpInitParams))
	_, _ = stdin.Write(jsonRPCMsg(nil, "notifications/initialized", nil))
	_, _ = stdin.Write(append(bytes.TrimRight(msg, "\n"), '\n'))

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	deadline := time.Now().Add(15 * time.Second)
	for sc.Scan() {
		if time.Now().After(deadline) {
			break
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var resp map[string]any
		if json.Unmarshal(line, &resp) != nil {
			continue
		}

		if fmt.Sprint(resp["id"]) == "1" {
			if res, ok := resp["result"].(map[string]any); ok {
				if _, hasCaps := res["capabilities"]; hasCaps {
					continue
				}
			}
		}
		if wantID == nil || fmt.Sprint(resp["id"]) == fmt.Sprint(wantID) {
			return line, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read mcp stdout: %w", err)
	}
	return nil, fmt.Errorf("no response for message within timeout")
}

func mcpGatewaySSEHandler(w http.ResponseWriter, r *http.Request, id string) {
	d, _ := store.Open()
	srv, err := store.GetMCPServer(d, id)
	if err != nil || srv == nil {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	if srv.Transport == "stdio" || srv.Transport == "" {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": "sse not available for stdio transport",
			"hint":  "use POST /api/mcp/" + id + "/message for stdio servers",
		})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	if _, verr := blockMetadataURL(r.Context(), srv.URL); verr != nil {
		http.Error(w, "blocked target (SSRF guard): "+verr.Error(), http.StatusForbidden)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	buf := make([]byte, 8192)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			return
		}
	}
}

func mcpListUpsertHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListMCPServers(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var m store.MCPServer
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if m.ID == "" || m.Name == "" {
			http.Error(w, "id + name required", http.StatusBadRequest)
			return
		}
		if err := store.UpsertMCPServer(d, &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func mcpCRUDHandler(w http.ResponseWriter, r *http.Request, id string) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		m, err := store.GetMCPServer(d, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if m == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, m)
	case http.MethodPut:
		var m store.MCPServer
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		m.ID = id
		if err := store.UpsertMCPServer(d, &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, m)
	case http.MethodDelete:
		if err := store.DeleteMCPServer(d, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func mcpToolsHandler(w http.ResponseWriter, r *http.Request, id string) {
	d, _ := store.Open()
	srv, err := store.GetMCPServer(d, id)
	if err != nil || srv == nil {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	tools, err := mcpListToolsLive(ctx, srv)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"server": id, "error": err.Error(), "transport": srv.Transport,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"server": id, "name": srv.Name, "transport": srv.Transport,
		"tools": tools, "count": len(tools),
	})
}

func mcpListToolsLive(ctx context.Context, srv *store.MCPServer) ([]map[string]any, error) {
	switch srv.Transport {
	case "stdio", "":
		return mcpStdioListTools(ctx, srv)
	case "http", "sse":
		return mcpHTTPListTools(ctx, srv)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", srv.Transport)
	}
}

func jsonRPCMsg(id any, method string, params any) []byte {
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		m["id"] = id
	}
	if params != nil {
		m["params"] = params
	}
	b, _ := json.Marshal(m)
	return append(b, '\n')
}

var mcpInitParams = map[string]any{
	"protocolVersion": "2024-11-05",
	"capabilities":    map[string]any{},
	"clientInfo":      map[string]any{"name": "flow_router", "version": "1.0"},
}

func mcpStdioListTools(ctx context.Context, srv *store.MCPServer) ([]map[string]any, error) {
	if srv.Command == "" {
		return nil, fmt.Errorf("stdio server missing command")
	}

	if !mcpsecurity.IsAllowed(srv.Command) {
		return nil, fmt.Errorf("command %q is not on the MCP allowlist", srv.Command)
	}
	if _, err := exec.LookPath(srv.Command); err != nil {
		return nil, fmt.Errorf("command %q not found on PATH", srv.Command)
	}
	cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)
	if len(srv.Env) > 0 {
		env := []string{}
		for k, v := range srv.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = append(cmd.Environ(), env...)
	}
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn: %w", err)
	}

	defer stdin.Close()
	defer stdout.Close()
	defer func() { _ = cmd.Process.Kill() }()

	_, _ = stdin.Write(jsonRPCMsg(1, "initialize", mcpInitParams))
	_, _ = stdin.Write(jsonRPCMsg(nil, "notifications/initialized", nil))
	_, _ = stdin.Write(jsonRPCMsg(2, "tools/list", map[string]any{}))

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	deadline := time.Now().Add(15 * time.Second)
	for scanner.Scan() {
		if time.Now().After(deadline) {
			break
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var resp struct {
			ID     any `json:"id"`
			Result struct {
				Tools []map[string]any `json:"tools"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &resp) != nil {
			continue
		}

		if fmt.Sprint(resp.ID) == "2" {
			if resp.Error != nil {
				return nil, fmt.Errorf("mcp error: %s", resp.Error.Message)
			}
			return resp.Result.Tools, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return nil, fmt.Errorf("no tools/list response within timeout")
}

func mcpHTTPListTools(ctx context.Context, srv *store.MCPServer) ([]map[string]any, error) {
	if srv.URL == "" {
		return nil, fmt.Errorf("http server missing url")
	}
	body := jsonRPCMsg(2, "tools/list", map[string]any{})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))

	payload := raw
	if bytes.Contains(raw, []byte("data:")) {
		for _, ln := range bytes.Split(raw, []byte("\n")) {
			if bytes.HasPrefix(ln, []byte("data:")) {
				payload = bytes.TrimSpace(bytes.TrimPrefix(ln, []byte("data:")))
			}
		}
	}
	var parsed struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("mcp error: %s", parsed.Error.Message)
	}
	return parsed.Result.Tools, nil
}
