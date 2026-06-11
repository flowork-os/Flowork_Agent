// ⚠️ EDITING? This is the deterministic NIGHTLY-SHUTDOWN operator. It does ONE thing:
// on a "/shutdown" message it calls the locked `system_power` builtin to power the
// host off. No LLM, no reasoning — a scheduler trigger at 00:00 invokes it. REAL power
// off only happens when the host is ARMED (env FLOWORK_POWER_ARMED=1); otherwise the
// tool dry-runs (safe on a dev machine). Keep it dumb and predictable.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

const respBufBytes = 1 << 20

var outBuf [respBufBytes]byte

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func emit(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

// loketCall reaches a kernel capability (here: tool.run) through the loket bus.
func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})
	reqJSON, _ := json.Marshal(map[string]any{
		"method": "POST", "url": loketURL, "timeout_ms": 30000, "max_resp_bytes": 1 << 20,
		"headers":     map[string]string{"Content-Type": "application/json"},
		"body_base64": base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return nil, fmt.Errorf("host_net_fetch: 0 bytes")
	}
	var host struct {
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:n], &host); err != nil {
		return nil, err
	}
	if host.Error != "" {
		return nil, fmt.Errorf("host: %s", host.Error)
	}
	raw, _ := base64.StdEncoding.DecodeString(host.BodyB64)
	var res struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, fmt.Errorf("loket refused: %s", res.Error)
	}
	return res.Result, nil
}

// msgText pulls the trigger/message text out of os.Args[2] ({"text":"..."}).
func msgText() string {
	if len(os.Args) < 3 {
		return ""
	}
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(os.Args[2]), &in)
	return strings.ToLower(strings.TrimSpace(in.Text))
}

// shutdown asks the locked system_power tool to power off the host after a short,
// cancellable delay. exec:power capability + FLOWORK_POWER_ARMED gate the real action.
func shutdown() {
	r, err := loketCall("tool.run", map[string]any{
		"name": "system_power",
		"args": map[string]any{
			"action":        "shutdown",
			"delay_seconds": 60, // 1-minute cancel window
			"reason":        "Scheduled nightly shutdown (00:00 WIB) — Flowork operator",
		},
	})
	if err != nil {
		emit(map[string]any{"agent": "operator-shutdown", "ok": false, "error": err.Error()})
		return
	}
	emit(map[string]any{"agent": "operator-shutdown", "ok": true, "result": r})
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case "handle_message", "handle", "shutdown":
		// Only ever power off on an explicit shutdown intent — never on a stray message.
		t := msgText()
		if os.Args[1] == "shutdown" || strings.Contains(t, "shutdown") || strings.Contains(t, "matikan") || strings.Contains(t, "/shutdown") {
			shutdown()
			return
		}
		emit(map[string]any{"agent": "operator-shutdown", "ok": true, "noop": "no shutdown intent in message"})
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}
