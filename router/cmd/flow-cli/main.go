// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	rootURL = ""
	apiKey  = ""
)

func main() {
	rootURL = envOr("FLOW_ROUTER_URL", "http://127.0.0.1:2402")
	apiKey = os.Getenv("FLOW_ROUTER_KEY")

	flag.StringVar(&rootURL, "url", rootURL, "flow_router base URL")
	flag.StringVar(&apiKey, "key", apiKey, "flow_router API key (Bearer)")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "status":
		exit(cmdStatus())
	case "providers":
		exit(cmdProviders())
	case "keys":
		exit(cmdKeys(rest))
	case "settings":
		exit(cmdSettings())
	case "tray":
		exit(cmdTray(rest))
	case "ui":
		exit(cmdUI(rest))
	case "autostart":
		exit(cmdAutostart(rest))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `flow-cli — control flow_router

USAGE:
  flow-cli [--url URL] [--key KEY] <command> [args]

COMMANDS:
  status                 Print router version + uptime + auth status
  providers              List providers (id, name, active)
  keys [list|new <name>] List API keys, or create a new one
  settings               Print full settings JSON
  ui                     Open the interactive menu (providers/keys/combos/...)
  tray [action]          Launch per-OS tray helper (status|open|restart)
  autostart {enable|disable|status}  Per-OS login autostart entry

ENV:
  FLOW_ROUTER_URL        Default --url (fallback http://127.0.0.1:2402)
  FLOW_ROUTER_KEY        Default --key (Bearer token)
`)
}

func cmdStatus() error {
	body, err := apiGET("/api/health")
	if err != nil {
		return err
	}
	var h struct {
		Service string `json:"service"`
		Status  string `json:"status"`
		Version string `json:"version"`
		Uptime  int64  `json:"uptime"`
	}
	_ = json.Unmarshal(body, &h)
	fmt.Printf("service:   %s\n", h.Service)
	fmt.Printf("status:    %s\n", h.Status)
	fmt.Printf("version:   %s\n", h.Version)
	fmt.Printf("uptime:    %ds\n", h.Uptime)
	if apiKey != "" {
		auth, _ := apiGET("/api/auth/status")
		fmt.Printf("auth:      %s\n", strings.TrimSpace(string(auth)))
	}
	return nil
}

func cmdProviders() error {
	body, err := apiGET("/api/providers")
	if err != nil {
		return err
	}
	var arr []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
		IsActive bool   `json:"isActive"`
		Priority int    `json:"priority"`
	}
	if err := json.Unmarshal(body, &arr); err != nil {

		var wrap struct {
			Data []struct {
				ID, Name, Provider string
				IsActive           bool
				Priority           int
			} `json:"data"`
		}
		if err2 := json.Unmarshal(body, &wrap); err2 != nil {
			return fmt.Errorf("parse providers: %w (raw: %s)", err, head(body))
		}
		fmt.Printf("%-36s  %-20s  %-10s  %-6s  %s\n", "id", "name", "provider", "active", "prio")
		for _, p := range wrap.Data {
			fmt.Printf("%-36s  %-20s  %-10s  %-6v  %d\n", p.ID, p.Name, p.Provider, p.IsActive, p.Priority)
		}
		return nil
	}
	fmt.Printf("%-36s  %-20s  %-10s  %-6s  %s\n", "id", "name", "provider", "active", "prio")
	for _, p := range arr {
		fmt.Printf("%-36s  %-20s  %-10s  %-6v  %d\n", p.ID, p.Name, p.Provider, p.IsActive, p.Priority)
	}
	return nil
}

func cmdKeys(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		body, err := apiGET("/api/keys")
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	}
	if args[0] == "new" {
		if len(args) < 2 {
			return fmt.Errorf("usage: flow-cli keys new <name>")
		}
		name := args[1]
		req := map[string]any{"name": name, "isActive": true}
		body, err := apiPOST("/api/keys", req)
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	}
	return fmt.Errorf("unknown keys sub-command: %s", args[0])
}

func cmdSettings() error {
	body, err := apiGET("/api/settings")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err == nil {
		fmt.Println(buf.String())
		return nil
	}
	fmt.Println(string(body))
	return nil
}

var httpc = &http.Client{Timeout: 15 * time.Second}

func apiGET(path string) ([]byte, error) {
	req, _ := http.NewRequest("GET", rootURL+path, nil)
	addAuth(req)
	return doReq(req)
}

func apiPOST(path string, body any) ([]byte, error) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", rootURL+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	addAuth(req)
	return doReq(req)
}

func addAuth(req *http.Request) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

func doReq(req *http.Request) ([]byte, error) {
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s -> %d: %s", req.Method, req.URL.Path, resp.StatusCode, head(body))
	}
	return body, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func head(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}
