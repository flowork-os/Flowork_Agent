// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&deathLetterWriteTool{})
	tools.Register(&factRecallTool{})
	tools.Register(&factWriteTool{})
	tools.Register(&askUserTool{})
}

type deathLetterWriteTool struct{}

func (deathLetterWriteTool) Name() string       { return "death_letter_write" }
func (deathLetterWriteTool) Capability() string { return "state:write" }
func (deathLetterWriteTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Tulis wasiat — pesan terakhir warga AI sebelum retire/upgrade/context-full. Warga baru di workspace sama auto-baca via Predecessor Honor Protocol (ADR-010). Subject + body required, recipient default 'all', letter_type default 'reflection'.",
		Params: []tools.Param{
			{Name: "subject", Type: tools.ParamString, Description: "Judul wasiat singkat", Required: true},
			{Name: "body", Type: tools.ParamString, Description: "Isi wasiat — markdown OK", Required: true},
			{Name: "letter_type", Type: tools.ParamString, Description: "reflection|handover|warning|legacy (default reflection)", Required: false},
			{Name: "recipient", Type: tools.ParamString, Description: "Penerima (default 'all')", Required: false},
		},
		Returns: "{ok, id} kalau sukses",
	}
}

func (deathLetterWriteTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available in ctx")
	}
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	letterType, _ := args["letter_type"].(string)
	recipient, _ := args["recipient"].(string)
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if subject == "" {
		return tools.Result{}, fmt.Errorf("subject required")
	}
	if body == "" {
		return tools.Result{}, fmt.Errorf("body required")
	}
	if letterType == "" {
		letterType = "reflection"
	}
	if recipient == "" {
		recipient = "all"
	}
	id, err := store.WriteLetter(letterType, recipient, subject, body)
	if err != nil {
		return tools.Result{}, fmt.Errorf("insert death letter: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id, "letter_type": letterType, "recipient": recipient},
		Note:   fmt.Sprintf("Death letter tertulis (id=%d, type=%s).", id, letterType),
	}, nil
}

type factRecallTool struct{}

func (factRecallTool) Name() string       { return "fact_recall" }
func (factRecallTool) Capability() string { return "state:read" }
func (factRecallTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Recall fact yang sebelumnya tersimpan via fact_write. Anti over-prompt: tool ini BUKAN auto-inject ke prompt — caller on-demand kalau perlu inget. Pakai key sebagai topic identifier.",
		Params: []tools.Param{
			{Name: "key", Type: tools.ParamString, Description: "Topic/fact key (case-sensitive)", Required: true},
		},
		Returns: "{key, value, found}",
	}
}

func (factRecallTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	key, _ := args["key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return tools.Result{}, fmt.Errorf("key required")
	}
	val, found, err := store.GetToolMemory(key)
	if err != nil {
		return tools.Result{}, fmt.Errorf("recall fact %q: %w", key, err)
	}
	if !found {
		return tools.Result{
			Output: map[string]any{"key": key, "found": false},
			Note:   fmt.Sprintf("Fact %q ngga ada di memory.", key),
		}, nil
	}
	return tools.Result{
		Output: map[string]any{"key": key, "value": val, "found": true},
	}, nil
}

type factWriteTool struct{}

func (factWriteTool) Name() string       { return "fact_write" }
func (factWriteTool) Capability() string { return "state:write" }
func (factWriteTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Simpan fact (key→value) ke memory. Idempotent upsert. Anti over-prompt: kalau fact essential, tulis dengan key descriptive (mis. 'owner_timezone') — caller panggil fact_recall(key) on-demand. Max 32KB per value (hard cap di DB layer).",
		Params: []tools.Param{
			{Name: "key", Type: tools.ParamString, Description: "Topic identifier (snake_case)", Required: true},
			{Name: "value", Type: tools.ParamString, Description: "Fact content. Max 32KB.", Required: true},
		},
		Returns: "{ok, key, value_len}",
	}
}

func (factWriteTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return tools.Result{}, fmt.Errorf("key required")
	}
	if value == "" {
		return tools.Result{}, fmt.Errorf("value required")
	}
	if err := store.SetToolMemory(key, value); err != nil {
		return tools.Result{}, fmt.Errorf("write fact %q: %w", key, err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "key": key, "value_len": len(value)},
		Note:   fmt.Sprintf("Fact %q tersimpan (%d chars).", key, len(value)),
	}, nil
}

type askUserTool struct{}

func (askUserTool) Name() string       { return "askuser" }
func (askUserTool) Capability() string { return "state:write" }
func (askUserTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Ask user untuk clarifikasi sebelum tindakan ambigu. LOG question + reasoning ke decisions table (decision_type='ask_clarification'). Caller (LLM) handle UI delivery via reply text. Anti over-tool: jangan panggil tiap step trivial. Pakai kalau input ambigu, multiple opsi tanpa hint, atau action irreversible.",
		Params: []tools.Param{
			{Name: "question", Type: tools.ParamString, Description: "Pertanyaan ke user (1 kalimat jelas)", Required: true},
			{Name: "reasoning", Type: tools.ParamString, Description: "Kenapa lo butuh tanya (1-2 kalimat)", Required: false},
		},
		Returns: "{ok, decision_id, question}",
	}
}

func (askUserTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	question, _ := args["question"].(string)
	reasoning, _ := args["reasoning"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return tools.Result{}, fmt.Errorf("question required")
	}
	inputs := map[string]any{"question": question, "reasoning": reasoning}
	id, err := store.LogDecision("ask_clarification", question, "pending", inputs, 0)
	if err != nil {
		return tools.Result{}, fmt.Errorf("log askuser decision: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "decision_id": id, "question": question},
		Note:   "Caller forward question ke user reply.",
	}, nil
}
