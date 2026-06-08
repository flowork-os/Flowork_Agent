// === LOCKED FILE ===
// Status: STABLE — computer-operator feature, tested end-to-end 2026-06-08. Deterministic computer-control executor (group member).
// Do not edit without owner approval.

// Package main is OPERATOR-KOMPUTER — a SIMPLE, deterministic executor ant.
//
// It is a GROUP member (operasi-komputer-grup). The group fans a task to it via
// the loket bus ("handle"); this ant parses the intent with plain rules (NO LLM,
// NO classifier, NO crew routing) and executes it through the engine's privileged
// tools over the loket:
//   - power  → system_power (shutdown/reboot/suspend/lock/logout/cancel + timer)
//   - apps   → app_open (chrome / vscode)
// Determinism is the point: "matiin pc 30 menit" must ALWAYS shut down, never get
// second-guessed or re-routed. Safety still lives in the tools (exec:power needs
// the privileged grant + FLOWORK_POWER_ARMED + a cancel window; app_open is
// allowlist-only). With nothing matched it replies with a short usage hint.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

var outBuf [262144]byte

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func emit(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }

func hostFetch(method, url string, headers map[string]string, body []byte) (int, []byte) {
	reqJSON, _ := json.Marshal(map[string]any{
		"method": method, "url": url, "timeout_ms": 65000, "max_resp_bytes": 4 << 20,
		"headers": headers, "body_base64": base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return 0, nil
	}
	var h struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(outBuf[:n], &h) != nil || h.Error != "" {
		return 0, nil
	}
	raw, _ := base64.StdEncoding.DecodeString(h.BodyB64)
	return h.Status, raw
}

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})
	st, raw := hostFetch("POST", loketURL, map[string]string{"Content-Type": "application/json"}, body)
	if st == 0 {
		return nil, fmt.Errorf("loket: no response")
	}
	var res struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil, fmt.Errorf("loket decode")
	}
	if !res.OK {
		return nil, fmt.Errorf("%s", res.Error)
	}
	return res.Result, nil
}

// toolRun executes one engine tool by name over the loket (cap tool.run). The
// tool's own fields (status, opened, …) may sit at the top level OR nested under
// "output" (tool.run wraps as {output,note}); resolveOut() handles both.
func toolRun(name string, args map[string]any) (json.RawMessage, error) {
	a, _ := json.Marshal(args)
	return loketCall("tool.run", map[string]any{"name": name, "args": json.RawMessage(a)})
}

// resolveOut digs out the object that holds the tool's own fields. The loket
// tool.run envelope is {ok, result:{output:{…}}, tool_name}; fall back to a bare
// {output:{…}} or the raw value so parsing is wrapping-agnostic.
func resolveOut(r json.RawMessage) json.RawMessage {
	var w struct {
		Result struct {
			Output json.RawMessage `json:"output"`
		} `json:"result"`
		Output json.RawMessage `json:"output"`
	}
	if json.Unmarshal(r, &w) == nil {
		if len(w.Result.Output) > 0 {
			return w.Result.Output
		}
		if len(w.Output) > 0 {
			return w.Output
		}
	}
	return r
}

var (
	reMinutes = regexp.MustCompile(`(\d+)\s*(menit|minute|min|m)\b`)
	reHours   = regexp.MustCompile(`(\d+)\s*(jam|hour|hr|h)\b`)
	reOpen    = regexp.MustCompile(`(?:buka|bukain|open)\s+([a-z0-9._-]+)`)
)

// delaySeconds extracts a timer ("30 menit", "1 jam") → seconds, capped at 3600
// (system_power's max). 0 = no explicit delay (tool uses its default window).
func delaySeconds(s string) int {
	if m := reHours.FindStringSubmatch(s); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 0 {
			d := n * 3600
			if d > 3600 {
				d = 3600
			}
			return d
		}
	}
	if m := reMinutes.FindStringSubmatch(s); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 0 {
			d := n * 60
			if d > 3600 {
				d = 3600
			}
			return d
		}
	}
	return 0
}

func has(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// execute maps a natural request to ONE deterministic tool call. Returns a human
// reply (Indonesian) describing what it did.
func execute(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return "kosong bro — contoh: 'matiin pc 30 menit', 'restart', 'buka chrome', 'kunci layar'."
	}

	// POWER actions FIRST (before app-open) so "reSTART" isn't mistaken for "start <app>".
	switch {
	case has(s, "batal", "cancel", "gajadi", "ga jadi", "jangan jadi", "urung"):
		r, err := toolRun("system_power", map[string]any{"action": "cancel"})
		return powerReply("cancel", 0, r, err)
	case has(s, "restart", "reboot", "mulai ulang", "booting ulang", "boot ulang", "start ulang"):
		d := delaySeconds(s)
		r, err := toolRun("system_power", actionArgs("reboot", d))
		return powerReply("reboot", d, r, err)
	case has(s, "matiin", "matikan", "shutdown", "shut down", "power off", "turn off", "matiin pc", "matikan pc"):
		d := delaySeconds(s)
		r, err := toolRun("system_power", actionArgs("shutdown", d))
		return powerReply("shutdown", d, r, err)
	case has(s, "suspend", "sleep", "tidur", "hibernate"):
		r, err := toolRun("system_power", map[string]any{"action": "suspend"})
		return powerReply("suspend", 0, r, err)
	case has(s, "kunci", "lock"):
		r, err := toolRun("system_power", map[string]any{"action": "lock"})
		return powerReply("lock", 0, r, err)
	case has(s, "logout", "log out", "sign out", "keluar akun"):
		r, err := toolRun("system_power", map[string]any{"action": "logout"})
		return powerReply("logout", 0, r, err)
	}
	// APP-OPEN last (specific keyword "buka"/"open" only — not greedy start/run).
	if m := reOpen.FindStringSubmatch(s); m != nil {
		app := strings.TrimSpace(m[1])
		r, err := toolRun("app_open", map[string]any{"app": app})
		return appReply(app, r, err)
	}
	return "ga ngerti perintahnya bro. yang gw bisa: matiin/restart/suspend/kunci/logout PC (+timer 'X menit'), batal, atau buka chrome/vscode."
}

func actionArgs(action string, delay int) map[string]any {
	a := map[string]any{"action": action}
	if delay > 0 {
		a["delay_seconds"] = delay
	}
	return a
}

func powerReply(action string, delay int, r json.RawMessage, err error) string {
	if err != nil {
		return "gagal bro: " + err.Error()
	}
	var o struct {
		Status string `json:"status"`
		Armed  bool   `json:"armed"`
		Msg    string `json:"message"`
	}
	_ = json.Unmarshal(resolveOut(r), &o)
	when := "sekarang"
	if delay > 0 {
		when = fmt.Sprintf("%d menit lagi", delay/60)
	}
	switch o.Status {
	case "scheduled":
		return fmt.Sprintf("✅ %s %s — ketik 'batal' kalau berubah pikiran.", labelID(action), when)
	case "cancelled":
		return "✅ dibatalin, ga jadi."
	case "nothing_pending":
		return "ga ada yang lagi nunggu buat dibatalin bro."
	case "dry_run":
		return fmt.Sprintf("⚠️ %s di-resolve tapi host belum ARMED (dry-run). Set FLOWORK_POWER_ARMED=1 buat eksekusi nyata.", labelID(action))
	default:
		if o.Msg != "" {
			return o.Msg
		}
		return fmt.Sprintf("oke, %s.", labelID(action))
	}
}

func appReply(app string, r json.RawMessage, err error) string {
	if err != nil {
		return "gagal buka " + app + ": " + err.Error()
	}
	var o struct {
		Opened  bool     `json:"opened"`
		App     string   `json:"app"`
		Error   string   `json:"error"`
		Allowed []string `json:"allowed"`
	}
	_ = json.Unmarshal(resolveOut(r), &o)
	if o.Opened {
		return "✅ " + o.App + " kebuka di komputer."
	}
	if len(o.Allowed) > 0 {
		return "ga bisa buka '" + app + "' — yang ada di allowlist: " + strings.Join(o.Allowed, ", ") + "."
	}
	if o.Error != "" {
		return "ga bisa buka " + app + ": " + o.Error
	}
	return "ga bisa buka " + app + "."
}

func labelID(action string) string {
	switch action {
	case "shutdown":
		return "PC bakal mati"
	case "reboot":
		return "PC bakal restart"
	case "suspend":
		return "PC bakal tidur (suspend)"
	case "lock":
		return "layar dikunci"
	case "logout":
		return "logout"
	}
	return action
}

// doHandle parses the message (unwrapping a {payload:{text}} group/bus envelope)
// and executes, emitting {reply}.
func doHandle(argsRaw string) {
	// unwrap a loket/group envelope {payload:{...}} if present
	var env struct {
		Payload json.RawMessage `json:"payload"`
	}
	body := argsRaw
	if json.Unmarshal([]byte(argsRaw), &env) == nil && len(env.Payload) > 0 {
		body = string(env.Payload)
	}
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(body), &in)
	emit(map[string]any{"reply": execute(in.Text)})
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch os.Args[1] {
	case "boot":
		// not a daemon — group member, invoked on demand. exit clean (idle).
		emit(map[string]any{"ok": true, "status": "idle (executor; invoked via group)"})
	case "handle", "handle_message":
		doHandle(args)
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}
