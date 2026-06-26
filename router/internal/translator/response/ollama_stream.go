// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type ollamaToolAcc struct {
	ID   string
	Name string
	Args string
}

func TransformOpenAISSEToOllamaNDJSON(src io.Reader, dst io.Writer, model string) int {
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	chunks := 0
	tools := map[int]*ollamaToolAcc{}
	finishReason := ""
	sentDone := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[len("data:"):])
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			emitOllamaDone(dst, model, tools, finishReason)
			sentDone = true
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		chunks++
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if fr, _ := choice["finish_reason"].(string); fr != "" {
			finishReason = fr
		}
		if tcs, ok := delta["tool_calls"].([]any); ok {
			for _, t := range tcs {
				tc, _ := t.(map[string]any)
				idxF, _ := tc["index"].(float64)
				idx := int(idxF)
				acc, has := tools[idx]
				if !has {
					acc = &ollamaToolAcc{}
					tools[idx] = acc
				}
				if id, _ := tc["id"].(string); id != "" {
					acc.ID = id
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					if n, _ := fn["name"].(string); n != "" {
						acc.Name += n
					}
					if a, _ := fn["arguments"].(string); a != "" {
						acc.Args += a
					}
				}
			}
		}

		if c, _ := delta["content"].(string); c != "" {
			row := map[string]any{
				"model": model,
				"message": map[string]any{
					"role":    "assistant",
					"content": c,
				},
				"done": false,
			}
			writeOllamaRow(dst, row)
		}
	}

	if !sentDone {
		emitOllamaDone(dst, model, tools, finishReason)
	}
	return chunks
}

func emitOllamaDone(w io.Writer, model string, tools map[int]*ollamaToolAcc, finishReason string) {
	message := map[string]any{"role": "assistant", "content": ""}
	if len(tools) > 0 {
		calls := make([]map[string]any, 0, len(tools))

		maxIdx := -1
		for i := range tools {
			if i > maxIdx {
				maxIdx = i
			}
		}
		for i := 0; i <= maxIdx; i++ {
			acc, ok := tools[i]
			if !ok {
				continue
			}
			var args any
			if acc.Args != "" {
				if err := json.Unmarshal([]byte(acc.Args), &args); err != nil {
					args = map[string]any{}
				}
			} else {
				args = map[string]any{}
			}
			calls = append(calls, map[string]any{
				"function": map[string]any{
					"name":      acc.Name,
					"arguments": args,
				},
			})
		}
		if len(calls) > 0 {
			message["tool_calls"] = calls
		}
	}
	_ = finishReason
	writeOllamaRow(w, map[string]any{
		"model":   model,
		"message": message,
		"done":    true,
	})
}

func writeOllamaRow(w io.Writer, row map[string]any) {
	raw, err := json.Marshal(row)
	if err != nil {
		return
	}
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte{'\n'})
}

func TransformOpenAISSEToOllamaBytes(sseBody []byte, model string) []byte {
	src := bytes.NewReader(sseBody)
	var dst bytes.Buffer
	TransformOpenAISSEToOllamaNDJSON(src, &dst, model)
	return dst.Bytes()
}
