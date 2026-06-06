// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: CLI connector (Connections Phase 2) + the project's QC harness (chats the
//   agent over the real /api/kernel/rpc pipeline). Self-managed config; multi-OS.
//
// flowork-connect — the CLI CONNECTOR for the Connections family.
//
// A CLI can't be a wasm module (a terminal can't be driven from inside wazero), so
// — exactly as the Connections design says — the CLI connector is HOST-SIDE: a tiny
// standalone binary. It is still a dumb pipe like every other connector: it forwards
// a line of text to a target agent and prints the reply. ALL the thinking lives in
// the agent; this owns nothing but its own little config.
//
// It reaches the agent through the SAME message entry the live Telegram connector
// drives — the loaded module's handle_message — via the loopback-only kernel RPC
// endpoint (/api/kernel/rpc, no token needed on 127.0.0.1). That means a reply here
// is the SAME reply a Telegram user gets, which is exactly why this doubles as the
// project's automated QC harness (chat the agent over the real pipeline, no human).
//
// Self-managed config (owner's rule "every connector manages itself"): the connector
// keeps its OWN settings file in its OWN folder — target agent + base URL — and
// nothing central. Multi-OS: home dir resolved at runtime, paths via filepath only.
//
//	flowork-connect "halo, kamu siapa?"     # one-shot
//	echo "harga BTC?" | flowork-connect      # piped: one message per line
//	flowork-connect                          # interactive REPL (tty)
//	flowork-connect --agent mr-flow-next --base http://127.0.0.1:1987 --save
//
// Exit: 0 ok · 1 transport error.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// config is the connector's self-owned settings, stored in its own folder.
type config struct {
	Agent string `json:"agent"`
	Base  string `json:"base"`
}

// configPath resolves the connector's OWN settings file. Override with
// FLOWORK_CONNECT_CONFIG; otherwise <home>/.flowork/connectors/cli/config.json.
// Built with filepath so it is correct on Windows/macOS/Linux alike.
func configPath() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_CONNECT_CONFIG")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".flowork", "connectors", "cli", "config.json")
}

func loadConfig() config {
	c := config{Agent: "mr-flow-next", Base: "http://127.0.0.1:1987"}
	raw, err := os.ReadFile(configPath())
	if err != nil {
		return c
	}
	var disk config
	if json.Unmarshal(raw, &disk) == nil {
		if disk.Agent != "" {
			c.Agent = disk.Agent
		}
		if disk.Base != "" {
			c.Base = disk.Base
		}
	}
	return c
}

func saveConfig(c config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	blob, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, blob, 0o644)
}

func main() {
	def := loadConfig()
	agent := flag.String("agent", def.Agent, "target agent id (the brain this CLI talks to)")
	base := flag.String("base", def.Base, "flowork-gui base URL (loopback)")
	asJSON := flag.Bool("json", false, "print the raw JSON reply")
	debug := flag.Bool("debug", false, "ask the agent for debug detail")
	save := flag.Bool("save", false, "persist --agent/--base to this connector's config, then continue")
	flag.Parse()

	if *save {
		if err := saveConfig(config{Agent: *agent, Base: *base}); err != nil {
			fmt.Fprintln(os.Stderr, "save config:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "saved → "+configPath())
	}

	c := &client{base: strings.TrimRight(*base, "/"), agent: *agent, asJSON: *asJSON, debug: *debug}

	// one-shot: everything after flags is a single message.
	if args := flag.Args(); len(args) > 0 {
		os.Exit(c.send(strings.Join(args, " ")))
	}

	// piped stdin: one message per line. Interactive tty: a REPL prompt.
	info, _ := os.Stdin.Stat()
	interactive := (info.Mode() & os.ModeCharDevice) != 0
	if interactive {
		fmt.Fprintf(os.Stderr, "flowork-connect → %s (%s). Ctrl-D to quit.\n", *agent, c.base)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	rc := 0
	for {
		if interactive {
			fmt.Fprint(os.Stderr, "› ")
		}
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if r := c.send(line); r != 0 {
			rc = r
		}
	}
	os.Exit(rc)
}

type client struct {
	base   string
	agent  string
	asJSON bool
	debug  bool
}

// send forwards one message to the agent via /api/kernel/rpc → handle_message and
// prints the reply. Returns a process exit code (0 ok, 1 transport error).
func (c *client) send(text string) int {
	payload, _ := json.Marshal(map[string]any{
		"plugin":   c.agent,
		"function": "handle_message",
		"args":     map[string]any{"text": text, "debug": c.debug},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/kernel/rpc", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "transport:", err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if c.asJSON {
		fmt.Println(string(body))
		return 0
	}
	var parsed struct {
		Reply string `json:"reply"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	switch {
	case parsed.Error != "":
		fmt.Fprintln(os.Stderr, "agent error:", parsed.Error)
		return 1
	case parsed.Reply != "":
		fmt.Println(parsed.Reply)
	default:
		fmt.Println(string(body)) // unknown shape — show raw so nothing is hidden
	}
	return 0
}
