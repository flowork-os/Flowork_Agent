// flowork-chat — CLI CHANNEL (roadmap Channels P2). Transport ke-3 (terminal)
// di atas core mr-flow channel-agnostic — SAMA kayak chat.go (HTTP) & daemon
// Telegram: stdin → amplop → mr-flow handle_message → stdout. NOL token/API
// eksternal → bukti pola generalisasi channel (transport ≠ inteligensi).
//
// Channel daftar (urut dibangun): telegram (daemon) · http (`/api/chat`) ·
// cli (INI). Semua manggil LOGIC yang sama → respons identik.
//
// Usage:
//
//	flowork-chat "halo kamu siapa"      # one-shot (arg)
//	echo "harga BTC?" | flowork-chat    # piped: tiap baris = 1 pesan
//	flowork-chat                        # REPL interaktif (tty)
//	flowork-chat --base http://127.0.0.1:1987 --user cli:owner --json
//
// Exit: 0 ok · 1 HTTP/network error.
//
// CATATAN: channel BUILT-IN (thin transport), bukan plugin kind=channel. Sama
// kayak chat.go — telegram-daemon→plugin removable = surgery bot LIVE (DEFER P1).

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
	"strings"
	"time"
)

func main() {
	base := flag.String("base", envOr("FLOWORK_BASE", "http://127.0.0.1:1987"), "flowork-gui base URL")
	user := flag.String("user", "cli:owner", "sender id (amplop)")
	asJSON := flag.Bool("json", false, "emit raw JSON response per message")
	flag.Parse()

	endpoint := strings.TrimRight(*base, "/") + "/api/chat"
	send := func(text string) int {
		text = strings.TrimSpace(text)
		if text == "" {
			return 0
		}
		reply, err := chatOnce(endpoint, text, *user, *asJSON)
		if err != nil {
			fmt.Fprintln(os.Stderr, "✕ "+err.Error())
			return 1
		}
		fmt.Println(reply)
		return 0
	}

	// 1) one-shot: pesan dari argumen.
	if args := flag.Args(); len(args) > 0 {
		os.Exit(send(strings.Join(args, " ")))
	}

	// 2) piped (stdin bukan tty): tiap baris = 1 pesan.
	if fi, _ := os.Stdin.Stat(); (fi.Mode() & os.ModeCharDevice) == 0 {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		rc := 0
		for sc.Scan() {
			if c := send(sc.Text()); c != 0 {
				rc = c
			}
		}
		os.Exit(rc)
	}

	// 3) REPL interaktif (tty).
	fmt.Fprintln(os.Stderr, "flowork-chat — ketik pesan, Ctrl-D buat keluar.")
	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "› ")
		if !sc.Scan() {
			fmt.Fprintln(os.Stderr)
			break
		}
		send(sc.Text())
	}
}

// chatOnce — POST /api/chat {text,user} → reply. Mirror chat.go contract.
func chatOnce(endpoint, text, user string, asJSON bool) (string, error) {
	body, _ := json.Marshal(map[string]string{"text": text, "user": user})
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if asJSON {
		return strings.TrimSpace(string(raw)), nil
	}
	var d struct {
		Reply string `json:"reply"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return "", fmt.Errorf("bad response (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if d.Error != "" {
		return "", fmt.Errorf("%s", d.Error)
	}
	return d.Reply, nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
