// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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
