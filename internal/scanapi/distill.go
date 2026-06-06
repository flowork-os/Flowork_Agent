// distill.go — GENERATOR check privat: LLM baca intel 5jt (SearchBrain) → tulis
// template nuclei → gerbang validate → ingest ke flowork-private. Sisi generator
// dari pipeline distilasi (sisi penerima = scanner_checks.go).
//
// AMAN (anti senjata): LLM DIPAKSA detection-only (http GET + matcher, forced-tool
// anti free-text halu); output di-tolak kalau ada protokol `code`/method
// destruktif; tiap template lewat `nuclei -validate`; nama di-sanitize. Owner-only.
//
//	POST /api/scanner/distill {topics[], model?}  → generate+validate+ingest batch

package scanapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"flowork-gui/internal/routerclient"
)

const distillModelDefault = "claude-haiku-4-5"

const distillSystemPrompt = `You are a senior web security detection engineer writing Nuclei v3 templates.
Produce ONE valid Nuclei template that DETECTS the given exposure/vulnerability on a target.

HARD RULES:
- "http" protocol ONLY. Method GET or HEAD. Use {{BaseURL}} for the target.
- Matchers: combine a "status" matcher with a "word" or "regex" matcher, and set "matchers-condition: and" to avoid false positives.
- DETECTION ONLY. NEVER use the "code" protocol, OS commands, or destructive methods (PUT/DELETE/POST that writes).
- Self-contained and syntactically valid for nuclei v3. info must have name + author: flowork + severity + tags.

Output STRICTLY by calling emit_template. The "yaml" field is the COMPLETE template text.

Valid example:
id: exposed-env
info:
  name: Exposed .env File
  author: flowork
  severity: high
  tags: exposure,config
http:
  - method: GET
    path:
      - "{{BaseURL}}/.env"
    matchers-condition: and
    matchers:
      - type: regex
        regex:
          - "(?m)^[A-Z0-9_]+=.+"
      - type: status
        status:
          - 200`

var distillTool = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "emit_template",
		"description": "Emit one complete, valid Nuclei v3 detection template.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "string", "description": "kebab-case template id, e.g. exposed-git-config"},
				"yaml": map[string]any{"type": "string", "description": "the COMPLETE nuclei v3 template YAML"},
			},
			"required": []any{"id", "yaml"},
		},
	},
}

// distillLLM — POST router forced-tool → balik arguments JSON. (Pola llm.go main,
// disalin biar generator nyatu di package scanapi.)
func distillLLM(ctx context.Context, model, system, user string) (json.RawMessage, error) {
	reqMap := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": user},
		},
		"tools":       []any{distillTool},
		"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "emit_template"}},
		"max_tokens":  1500,
	}
	body, _ := json.Marshal(reqMap)
	hreq, _ := http.NewRequestWithContext(ctx, "POST", routerclient.DefaultRouterURL+"/v1/chat/completions", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("router call: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("router status %d", resp.StatusCode)
	}
	var oResp struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &oResp); err != nil {
		return nil, err
	}
	if len(oResp.Choices) == 0 || len(oResp.Choices[0].Message.ToolCalls) == 0 {
		return nil, fmt.Errorf("LLM ga manggil emit_template")
	}
	return json.RawMessage(oResp.Choices[0].Message.ToolCalls[0].Function.Arguments), nil
}

// sanitizeCheckID — slug aman buat nama file ([a-z0-9._-]).
func sanitizeCheckID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// distillOne — 1 topik → template tervalidasi. SearchBrain (grounding 5jt) → LLM
// forced-tool → sanity safety → balik (id, yaml). err != nil = gagal generate.
func distillOne(ctx context.Context, model, topic string) (id, yaml string, err error) {
	intel := ""
	rc := routerclient.New(routerclient.DefaultRouterURL)
	if resp, e := rc.SearchBrain(ctx, topic, 3); e == nil {
		var b strings.Builder
		for _, h := range resp.Hits {
			c := strings.ReplaceAll(h.Content, "\n", " ")
			if len(c) > 500 {
				c = c[:500]
			}
			b.WriteString("- " + c + "\n")
		}
		intel = b.String()
	}
	user := "Exposure/vulnerability to detect: " + topic
	if intel != "" {
		user += "\n\nReference intel from our corpus:\n" + intel
	}
	args, err := distillLLM(ctx, model, distillSystemPrompt, user)
	if err != nil {
		return "", "", err
	}
	var out struct {
		ID   string `json:"id"`
		YAML string `json:"yaml"`
	}
	if e := json.Unmarshal(args, &out); e != nil {
		return "", "", fmt.Errorf("decode emit: %w", e)
	}
	id = sanitizeCheckID(out.ID)
	if id == "" || strings.TrimSpace(out.YAML) == "" {
		return "", "", fmt.Errorf("output LLM kosong/invalid")
	}
	// SAFETY: tolak protokol code / method destruktif (defense-in-depth, walau
	// runtime juga ga pernah pass -code).
	low := strings.ToLower(out.YAML)
	for _, bad := range []string{"code:", "method: delete", "method: put", "javascript:", "- engine:"} {
		if strings.Contains(low, bad) {
			return "", "", fmt.Errorf("ditolak safety: mengandung %q", bad)
		}
	}
	return id, out.YAML, nil
}

// ScannerDistillHandler — POST {topics[], model?}: generate+validate+ingest batch.
func ScannerDistillHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Topics []string `json:"topics"`
			Model  string   `json:"model"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		if len(body.Topics) == 0 {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "topics[] wajib"})
			return
		}
		model := strings.TrimSpace(body.Model)
		if model == "" {
			model = distillModelDefault
		}
		dir := privateChecksDir()
		if dir == "" {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dir template nuclei ga ketemu"})
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		type result struct {
			Topic  string `json:"topic"`
			Name   string `json:"name,omitempty"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		}
		results := make([]result, 0, len(body.Topics))
		added := 0
		for _, topic := range body.Topics {
			topic = strings.TrimSpace(topic)
			if topic == "" {
				continue
			}
			ctx, cancel := context.WithTimeout(r.Context(), 130*time.Second)
			id, yaml, gerr := distillOne(ctx, model, topic)
			if gerr != nil {
				cancel()
				results = append(results, result{Topic: topic, Status: "gen_fail", Error: trunc(gerr.Error(), 160)})
				continue
			}
			name := "flowork-" + id
			if verr := ingestValidatedCheck(dir, name, yaml); verr != nil {
				cancel()
				results = append(results, result{Topic: topic, Name: name, Status: "invalid", Error: trunc(verr.Error(), 160)})
				continue
			}
			cancel()
			added++
			results = append(results, result{Topic: topic, Name: name, Status: "added"})
		}
		if added > 0 {
			resetNucleiPackCache()
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "added": added, "total": len(body.Topics), "model": model, "results": results})
	}
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
