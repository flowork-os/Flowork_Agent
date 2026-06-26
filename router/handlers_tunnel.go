// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/safego"
	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	cloudflaredMu  sync.Mutex
	cloudflaredCmd *exec.Cmd
)

func tunnelStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	st, _ := store.LoadTunnelState(d)
	st.CloudflareEnabled = isCloudflaredRunning()
	if !st.CloudflareEnabled {
		st.CloudflareURL = ""
		st.CloudflarePID = 0
	}

	if _, err := exec.LookPath("tailscale"); err == nil {
		st.TailscaleInstalled = true
		if out, err := runShort("tailscale", "status", "--json"); err == nil {
			st.TailscaleEnabled = strings.Contains(out, `"BackendState":"Running"`)
			if mURL := extractTailscaleURL(out); mURL != "" {
				st.TailscaleURL = mURL
			}
		}
	}
	_ = store.SaveTunnelState(d, st)
	writeJSON(w, http.StatusOK, st)
}

func tunnelEnableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := exec.LookPath("cloudflared"); err != nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": "cloudflared binary not found",
			"hint":  "install: curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-" + runtime.GOOS + "-" + runtime.GOARCH + " -o /usr/local/bin/cloudflared && chmod +x /usr/local/bin/cloudflared",
		})
		return
	}

	d, derr := store.Open()
	if derr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "cannot verify auth settings before tunnel: " + derr.Error(),
		})
		return
	}
	if st, _ := store.LoadSettings(d); st == nil || !st.RequireLogin || st.AuthMode == "none" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "refusing to start tunnel: login is not enforced",
			"hint":  "enable RequireLogin with a password/OIDC auth mode first (Settings → Security) — a tunnel would otherwise expose the admin API unauthenticated to the internet",
		})
		return
	}
	cloudflaredMu.Lock()
	defer cloudflaredMu.Unlock()
	if cloudflaredCmd != nil && cloudflaredCmd.Process != nil {

		d, _ := store.Open()
		st, _ := store.LoadTunnelState(d)
		writeJSON(w, http.StatusOK, map[string]any{
			"alreadyRunning": true,
			"state":          st,
		})
		return
	}
	var body struct {
		Port int `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Port == 0 {
		body.Port = 2402
	}
	if body.Port < 1 || body.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "port out of range (1-65535)"})
		return
	}
	target := fmt.Sprintf("http://127.0.0.1:%d", body.Port)
	cmd := exec.Command("cloudflared", "tunnel", "--no-autoupdate", "--url", target)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {

		if stdoutPipe != nil {
			_ = stdoutPipe.Close()
		}
		if stderrPipe != nil {
			_ = stderrPipe.Close()
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "spawn cloudflared"})
		return
	}
	cloudflaredCmd = cmd

	urlCh := make(chan string, 1)
	safego.GoLabel("cloudflared-stdout", func() { scanCloudflaredOutput(stdoutPipe, urlCh) })
	safego.GoLabel("cloudflared-stderr", func() { scanCloudflaredOutput(stderrPipe, urlCh) })
	safego.GoLabel("cloudflared-wait", func() {
		_ = cmd.Wait()
		cloudflaredMu.Lock()
		cloudflaredCmd = nil
		cloudflaredMu.Unlock()
		d, _ := store.Open()
		st, _ := store.LoadTunnelState(d)
		st.CloudflareEnabled = false
		st.CloudflareURL = ""
		st.CloudflarePID = 0
		_ = store.SaveTunnelState(d, st)
	})

	d, _ = store.Open()
	st, _ := store.LoadTunnelState(d)
	st.CloudflareEnabled = true
	st.CloudflarePID = cmd.Process.Pid
	_ = store.SaveTunnelState(d, st)

	select {
	case u := <-urlCh:
		st.CloudflareURL = u
		_ = store.SaveTunnelState(d, st)
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": true,
			"url":     u,
			"pid":     cmd.Process.Pid,
		})
	case <-time.After(15 * time.Second):
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": true,
			"pid":     cmd.Process.Pid,
			"note":    "URL not yet detected — call /api/tunnel/status",
		})
	}
}

func tunnelDisableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cloudflaredMu.Lock()
	defer cloudflaredMu.Unlock()
	if cloudflaredCmd == nil || cloudflaredCmd.Process == nil {
		writeJSON(w, http.StatusOK, map[string]any{"disabled": true, "wasRunning": false})
		return
	}
	pid := cloudflaredCmd.Process.Pid
	_ = cloudflaredCmd.Process.Kill()
	cloudflaredCmd = nil
	d, _ := store.Open()
	st, _ := store.LoadTunnelState(d)
	st.CloudflareEnabled = false
	st.CloudflareURL = ""
	st.CloudflarePID = 0
	_ = store.SaveTunnelState(d, st)
	writeJSON(w, http.StatusOK, map[string]any{"disabled": true, "killedPid": pid})
}

func tailscaleCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out := map[string]any{}
	if _, err := exec.LookPath("tailscale"); err != nil {
		out["installed"] = false
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["installed"] = true
	if statusJSON, err := runShort("tailscale", "status", "--json"); err == nil {
		out["enabled"] = strings.Contains(statusJSON, `"BackendState":"Running"`)
		if u := extractTailscaleURL(statusJSON); u != "" {
			out["url"] = u
		}
		out["raw"] = statusJSON[:min(2000, len(statusJSON))]
	} else {
		out["enabled"] = false
		out["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

func tailscaleInstallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var instructions string
	switch runtime.GOOS {
	case "linux":
		instructions = "curl -fsSL https://tailscale.com/install.sh | sh"
	case "darwin":
		instructions = "brew install tailscale  # or download from https://tailscale.com/download/mac"
	default:
		instructions = "see https://tailscale.com/download for your OS"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requiresManualSudo": true,
		"command":            instructions,
		"reason":             "flow_router does not invoke sudo on user's behalf — paste the command into a shell",
	})
}

func tailscaleEnableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := exec.LookPath("tailscale"); err != nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "tailscale not installed"})
		return
	}
	out, err := runShort("tailscale", "up", "--accept-routes", "--accept-dns=true")
	resp := map[string]any{"output": out}
	if err != nil {
		resp["error"] = err.Error()

		if u := extractAuthURL(out); u != "" {
			resp["authUrl"] = u
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if u := extractAuthURL(out); u != "" {
		resp["authUrl"] = u
	}
	resp["enabled"] = true
	writeJSON(w, http.StatusOK, resp)
}

func tailscaleDisableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := exec.LookPath("tailscale"); err != nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "tailscale not installed"})
		return
	}
	out, err := runShort("tailscale", "down")
	resp := map[string]any{"output": out}
	if err != nil {
		resp["error"] = err.Error()
	} else {
		resp["disabled"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

func isCloudflaredRunning() bool {
	cloudflaredMu.Lock()
	defer cloudflaredMu.Unlock()
	if cloudflaredCmd == nil || cloudflaredCmd.Process == nil {
		return false
	}

	if cloudflaredCmd.ProcessState != nil && cloudflaredCmd.ProcessState.Exited() {
		return false
	}
	return true
}

var (
	cloudflareURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
	authURLRe       = regexp.MustCompile(`https://login\.tailscale\.com/[^\s]+`)
)

func scanCloudflaredOutput(r io.ReadCloser, urlCh chan<- string) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if m := cloudflareURLRe.FindString(line); m != "" {
			select {
			case urlCh <- m:
			default:
			}
		}
	}
	_ = sc.Err()
}

func extractTailscaleURL(s string) string {

	m := regexp.MustCompile(`"TailscaleIPs":\s*\[\s*"([^"]+)"`).FindStringSubmatch(s)
	if len(m) > 1 {
		return "http://" + m[1] + ":2402"
	}
	return ""
}

func extractAuthURL(s string) string {
	return authURLRe.FindString(s)
}

func runShort(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}
