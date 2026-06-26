// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type ResponsesSSEWriter struct {
	w          io.Writer
	flushFn    func()
	mu         sync.Mutex
	responseID string
	model      string
	created    int64
	seq        int

	msgItemID     string
	msgItemAdded  bool
	textPartAdded bool
	contentBuf    strings.Builder

	reasonItemID    string
	reasonItemAdded bool
	reasonPartAdded bool
	reasonBuf       strings.Builder

	toolItems map[int]*responsesToolItem

	completedSent bool
	finishReason  string
	usage         map[string]any
}

type responsesToolItem struct {
	ID        string
	Name      string
	ItemAdded bool
	ArgsBuf   strings.Builder
}

func NewResponsesSSEWriter(w io.Writer, flush func(), model string) *ResponsesSSEWriter {
	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	r := &ResponsesSSEWriter{
		w:          w,
		flushFn:    flush,
		responseID: respID,
		model:      model,
		created:    time.Now().Unix(),
		toolItems:  map[int]*responsesToolItem{},
	}
	r.emit("response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         respID,
			"object":     "response",
			"created_at": r.created,
			"model":      model,
			"status":     "in_progress",
		},
	})
	return r
}

func (r *ResponsesSSEWriter) Feed(chunk map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		if u, ok := chunk["usage"].(map[string]any); ok {
			r.usage = u
		}
		return
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)

	if rc, _ := delta["reasoning_content"].(string); rc != "" {
		r.emitReasoningDelta(rc)
	}

	if c, _ := delta["content"].(string); c != "" {
		r.emitTextDelta(c)
	}

	if tcs, ok := delta["tool_calls"].([]any); ok {
		for _, t := range tcs {
			tc, _ := t.(map[string]any)
			r.emitToolDelta(tc)
		}
	}

	if fr, _ := choice["finish_reason"].(string); fr != "" {
		r.finishReason = fr
	}
	if u, ok := chunk["usage"].(map[string]any); ok {
		r.usage = u
	}
}

func (r *ResponsesSSEWriter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completedSent {
		return
	}
	r.completedSent = true

	if r.textPartAdded {
		r.emit("response.output_text.done", map[string]any{
			"type":          "response.output_text.done",
			"item_id":       r.msgItemID,
			"output_index":  0,
			"content_index": 0,
			"text":          r.contentBuf.String(),
		})
		r.emit("response.content_part.done", map[string]any{
			"type":          "response.content_part.done",
			"item_id":       r.msgItemID,
			"output_index":  0,
			"content_index": 0,
			"part":          map[string]any{"type": "output_text", "text": r.contentBuf.String()},
		})
	}
	if r.msgItemAdded {
		r.emit("response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": 0,
			"item": map[string]any{
				"id":     r.msgItemID,
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{{
					"type": "output_text",
					"text": r.contentBuf.String(),
				}},
			},
		})
	}

	if r.reasonItemAdded {
		r.emit("response.reasoning_summary_text.done", map[string]any{
			"type":          "response.reasoning_summary_text.done",
			"item_id":       r.reasonItemID,
			"output_index":  1,
			"summary_index": 0,
			"text":          r.reasonBuf.String(),
		})
	}

	for idx, tool := range r.toolItems {
		if !tool.ItemAdded {
			continue
		}
		r.emit("response.function_call_arguments.done", map[string]any{
			"type":         "response.function_call_arguments.done",
			"item_id":      tool.ID,
			"output_index": idx + 2,
			"arguments":    tool.ArgsBuf.String(),
		})
	}

	final := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":         r.responseID,
			"object":     "response",
			"created_at": r.created,
			"model":      r.model,
			"status":     "completed",
		},
	}
	if r.usage != nil {
		final["response"].(map[string]any)["usage"] = r.usage
	}
	r.emit("response.completed", final)
}

func (r *ResponsesSSEWriter) emitTextDelta(text string) {
	if !r.msgItemAdded {
		r.msgItemID = "msg_" + r.responseID
		r.emit("response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": 0,
			"item": map[string]any{
				"id":     r.msgItemID,
				"type":   "message",
				"role":   "assistant",
				"status": "in_progress",
			},
		})
		r.msgItemAdded = true
	}
	if !r.textPartAdded {
		r.emit("response.content_part.added", map[string]any{
			"type":          "response.content_part.added",
			"item_id":       r.msgItemID,
			"output_index":  0,
			"content_index": 0,
			"part":          map[string]any{"type": "output_text", "text": ""},
		})
		r.textPartAdded = true
	}
	r.contentBuf.WriteString(text)
	r.emit("response.output_text.delta", map[string]any{
		"type":          "response.output_text.delta",
		"item_id":       r.msgItemID,
		"output_index":  0,
		"content_index": 0,
		"delta":         text,
	})
}

func (r *ResponsesSSEWriter) emitReasoningDelta(text string) {
	if !r.reasonItemAdded {
		r.reasonItemID = "rs_" + r.responseID
		r.emit("response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": 1,
			"item": map[string]any{
				"id":      r.reasonItemID,
				"type":    "reasoning",
				"summary": []any{},
			},
		})
		r.reasonItemAdded = true
	}
	if !r.reasonPartAdded {
		r.emit("response.reasoning_summary_part.added", map[string]any{
			"type":          "response.reasoning_summary_part.added",
			"item_id":       r.reasonItemID,
			"output_index":  1,
			"summary_index": 0,
			"part":          map[string]any{"type": "summary_text", "text": ""},
		})
		r.reasonPartAdded = true
	}
	r.reasonBuf.WriteString(text)
	r.emit("response.reasoning_summary_text.delta", map[string]any{
		"type":          "response.reasoning_summary_text.delta",
		"item_id":       r.reasonItemID,
		"output_index":  1,
		"summary_index": 0,
		"delta":         text,
	})
}

func (r *ResponsesSSEWriter) emitToolDelta(tc map[string]any) {
	idxF, _ := tc["index"].(float64)
	idx := int(idxF)
	tool, ok := r.toolItems[idx]
	if !ok {
		tool = &responsesToolItem{}
		r.toolItems[idx] = tool
	}
	if id, _ := tc["id"].(string); id != "" {
		tool.ID = id
	}
	fn, _ := tc["function"].(map[string]any)
	if name, _ := fn["name"].(string); name != "" {
		tool.Name += name
	}

	if !tool.ItemAdded {
		if tool.ID == "" {
			tool.ID = fmt.Sprintf("fc_%s_%d", r.responseID, idx)
		}
		r.emit("response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": idx + 2,
			"item": map[string]any{
				"id":     tool.ID,
				"type":   "function_call",
				"status": "in_progress",
				"name":   tool.Name,
			},
		})
		tool.ItemAdded = true
	}

	if args, _ := fn["arguments"].(string); args != "" {
		tool.ArgsBuf.WriteString(args)
		r.emit("response.function_call_arguments.delta", map[string]any{
			"type":         "response.function_call_arguments.delta",
			"item_id":      tool.ID,
			"output_index": idx + 2,
			"delta":        args,
		})
	}
}

func (r *ResponsesSSEWriter) emit(event string, data map[string]any) {
	r.seq++
	data["sequence_number"] = r.seq
	raw, _ := json.Marshal(data)
	fmt.Fprintf(r.w, "event: %s\ndata: %s\n\n", event, raw)
	if r.flushFn != nil {
		r.flushFn()
	}
}
