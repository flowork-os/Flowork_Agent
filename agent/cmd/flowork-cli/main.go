// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	defaultAgent   = "mr-flow"
	defaultBase    = "http://127.0.0.1:1987"
	defaultCaller  = "flowork-cli"
	defaultTimeout = 30 * time.Second
)

func main() {
	agent := flag.String("agent", defaultAgent, "agent id target")
	base := flag.String("base", defaultBase, "daemon base URL")
	caller := flag.String("caller", defaultCaller, "caller identity (audit log)")
	jsonOut := flag.Bool("json", false, "raw JSON response output")
	repl := flag.Bool("repl", false, "interactive shell (multi-command session)")
	timeout := flag.Duration("timeout", defaultTimeout, "request timeout")
	flag.Parse()

	if *repl {
		runREPL(*base, *agent, *caller, *jsonOut, *timeout)
		return
	}

	text := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if text == "" {
		fmt.Fprintln(os.Stderr, "usage: flowork-cli [flags] /<command> [args...]")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if !strings.HasPrefix(text, "/") {
		fmt.Fprintf(os.Stderr, "slash command must start with '/'\n")
		os.Exit(2)
	}
	rc := runOnce(*base, *agent, *caller, text, *jsonOut, *timeout)
	os.Exit(rc)
}

func runOnce(base, agent, caller, text string, jsonOut bool, timeout time.Duration) int {
	resp, err := dispatch(base, agent, caller, text, timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if jsonOut {
		raw, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(raw))
		return 0
	}

	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "slash error: %s\n", resp.Error)
		return 2
	}
	if resp.Result.Text != "" {
		fmt.Println(resp.Result.Text)
	} else {
		fmt.Println("(no output)")
	}
	if resp.DurationMS > 0 {
		fmt.Fprintf(os.Stderr, "[%s in %dms]\n", resp.Command, resp.DurationMS)
	}
	return 0
}

func runREPL(base, agent, caller string, jsonOut bool, timeout time.Duration) {
	fmt.Printf("flowork-cli REPL — agent=%s base=%s. /exit untuk keluar.\n",
		agent, base)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; fmt.Println("\nbye."); os.Exit(0) }()

	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("(%s)> ", agent)
		line, err := in.ReadString('\n')
		if err == io.EOF {
			fmt.Println()
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "read: %v\n", err)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "/exit" || line == "/quit" {
			return
		}
		if !strings.HasPrefix(line, "/") {
			fmt.Fprintln(os.Stderr, "(commands must start with '/')")
			continue
		}
		_ = runOnce(base, agent, caller, line, jsonOut, timeout)
	}
}

type slashResp struct {
	Command    string `json:"command"`
	DurationMS int64  `json:"duration_ms"`
	OK         bool   `json:"ok"`
	Result     struct {
		Text   string `json:"text"`
		Format string `json:"format"`
	} `json:"result"`
	Error string `json:"error"`
}

func dispatch(base, agent, caller, text string, timeout time.Duration) (*slashResp, error) {
	body, _ := json.Marshal(map[string]string{
		"text":   text,
		"caller": caller,
	})
	url := strings.TrimRight(base, "/") + "/api/agents/slash/run?id=" + agent
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: timeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("daemon %d: %s", resp.StatusCode, string(raw))
	}
	var out slashResp
	if jerr := json.Unmarshal(raw, &out); jerr != nil {
		return nil, fmt.Errorf("decode: %w (raw=%q)", jerr, string(raw))
	}
	return &out, nil
}
