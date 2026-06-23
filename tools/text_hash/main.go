// text_hash — TOOL SIDECAR contoh (self-contained, plug-and-play).
//
// Prinsip owner: tool = folder sendiri, dependency di folder sendiri (NOL shared lib), di-compile
// jadi binary native sendiri. ABI: baca JSON {"args":{...}} dari STDIN, balikin {"output":..,"error":..}
// ke STDOUT. Ini contoh stdlib-only; tool yg butuh lib eksternal taruh di go.mod + vendor FOLDER INI.
package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
)

func main() {
	var req struct {
		Args struct {
			Text string `json:"text"`
			Algo string `json:"algo"`
		} `json:"args"`
	}
	in, _ := io.ReadAll(os.Stdin)
	_ = json.Unmarshal(in, &req)
	text := req.Args.Text
	if text == "" {
		emit(nil, "param 'text' wajib")
		return
	}
	algo := req.Args.Algo
	var sum []byte
	switch algo {
	case "md5":
		s := md5.Sum([]byte(text))
		sum = s[:]
	case "sha1":
		s := sha1.Sum([]byte(text))
		sum = s[:]
	default:
		algo = "sha256"
		s := sha256.Sum256([]byte(text))
		sum = s[:]
	}
	emit(map[string]any{"algo": algo, "hash": hex.EncodeToString(sum), "len": len(text)}, "")
}

func emit(output any, errStr string) {
	b, _ := json.Marshal(map[string]any{"output": output, "error": errStr})
	_, _ = os.Stdout.Write(b)
}
