// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: SDK sample. Audit pass — args bounds checked, json error handled,
//   empty input handled. Sample-only (not runtime), ABI pattern reference.
//
// echo — minimum Flowork agent sample.
//
// ABI (command pattern): kernel invoke binary dengan
//
//	os.Args[0] = "agent"
//	os.Args[1] = <function_name>          e.g. "echo"
//	os.Args[2] = <args_json>              e.g. `{"text":"hi"}`
//
// Agent tulis JSON response ke stdout, exit. Tidak ada export wasmexport
// karena TinyGo wasmexport panic setelah _start exit.
//
// Build dari root repo: ./scripts/build-agent.sh echo
// (Source harus pindah ke agents/echo/ dulu sebelum jalan script.)

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		emit(map[string]string{"error": "missing function"})
		return
	}
	fn := os.Args[1]
	argsRaw := ""
	if len(os.Args) >= 3 {
		argsRaw = os.Args[2]
	}
	switch fn {
	case "echo":
		doEcho(argsRaw)
	default:
		emit(map[string]string{"error": "unknown function: " + fn})
	}
}

func doEcho(argsRaw string) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(argsRaw), &in); err != nil {
		emit(map[string]any{"reply": "parse: " + err.Error()})
		return
	}
	if in.Text == "" {
		emit(map[string]any{"reply": "(empty)"})
		return
	}
	emit(map[string]any{
		"reply": fmt.Sprintf("[%s] %s", time.Now().UTC().Format(time.RFC3339), in.Text),
	})
}

func emit(v any) {
	body, _ := json.Marshal(v)
	fmt.Println(string(body))
}
