package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/factmemory"
	"github.com/teetah2402/flowork/internal/provider"
)

// FactRememberTool — agent simpan satu triplet fakta ke persistent fact memory.
// Contoh: fact_remember(subject="Ayah", predicate="pakai", object="Windows 11")
type FactRememberTool struct{ workspace string }

func NewFactRememberTool(workspace string) *FactRememberTool {
	return &FactRememberTool{workspace}
}

func (t *FactRememberTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "fact_remember",
		Description: "Simpan fakta tentang entitas ke persistent fact memory lintas sesi. Format: subject (siapa/apa), predicate (kata kerja/relasi), object (nilai/deskripsi). Contoh: Ayah pakai Windows 11.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject":    map[string]any{"type": "string", "description": "Entitas. Misal: 'Ayah', 'Flowork', 'tim'"},
				"predicate":  map[string]any{"type": "string", "description": "Relasi. Misal: 'pakai', 'adalah', 'punya', 'tinggal di'"},
				"object":     map[string]any{"type": "string", "description": "Nilai fakta. Misal: 'Windows 11', 'bedridden founder'"},
				"confidence": map[string]any{"type": "number", "description": "Keyakinan 0.0-1.0 (default 0.8)"},
			},
			"required": []string{"subject", "predicate", "object"},
		},
	}
}

func (t *FactRememberTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	subject, _ := inv.ParsedArgs["subject"].(string)
	predicate, _ := inv.ParsedArgs["predicate"].(string)
	object, _ := inv.ParsedArgs["object"].(string)
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(predicate) == "" {
		return Result{ToolName: inv.ToolName, OK: false, Output: "subject dan predicate wajib diisi"}, nil
	}

	conf := 0.8
	if c, ok := inv.ParsedArgs["confidence"].(float64); ok && c > 0 {
		conf = c
	}

	lib, err := factmemory.Open(t.workspace)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("fact memory error: %v", err)}, nil
	}
	fact := factmemory.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     "agent",
		Confidence: conf,
	}
	id, err := lib.Add(fact)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("gagal simpan: %v", err)}, nil
	}
	return Result{
		ToolName: inv.ToolName,
		OK:       true,
		Output:   fmt.Sprintf("✓ fact[%s]: %s %s %s (conf=%.1f)", id, subject, predicate, object, conf),
	}, nil
}

// FactRecallTool — query fact memory by free-text query.
type FactRecallTool struct{ workspace string }

func NewFactRecallTool(workspace string) *FactRecallTool { return &FactRecallTool{workspace} }

func (t *FactRecallTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "fact_recall",
		Description: "Cari fakta relevan dari fact memory. Gunakan untuk ingat preferensi Ayah, konteks proyek, atau informasi yang disimpan lintas sesi.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Topik atau pertanyaan. Misal: 'Ayah OS', 'team size', 'wallet address'"},
				"k":     map[string]any{"type": "integer", "description": "Jumlah hasil max, default 5"},
			},
			"required": []string{"query"},
		},
	}
}

func (t *FactRecallTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	query, _ := inv.ParsedArgs["query"].(string)
	if strings.TrimSpace(query) == "" {
		return Result{ToolName: inv.ToolName, OK: false, Output: "query wajib diisi"}, nil
	}
	k := 5
	if kv, ok := inv.ParsedArgs["k"].(float64); ok && kv > 0 {
		k = int(kv)
	}

	lib, err := factmemory.Open(t.workspace)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("fact memory error: %v", err)}, nil
	}
	if lib.Count() == 0 {
		return Result{ToolName: inv.ToolName, OK: true, Output: "Fact memory kosong — belum ada fakta tersimpan."}, nil
	}
	results := lib.Query(query, k)
	if len(results) == 0 {
		return Result{ToolName: inv.ToolName, OK: true, Output: fmt.Sprintf("Tidak ada fakta relevan untuk query: %q", query)}, nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Fact recall (%d hasil untuk %q):\n", len(results), query)
	for _, r := range results {
		fmt.Fprintf(&sb, "  [%.2f] %s %s %s\n", r.Score, r.Fact.Subject, r.Fact.Predicate, r.Fact.Object)
	}
	return Result{ToolName: inv.ToolName, OK: true, Output: sb.String()}, nil
}

// FactForgetTool — hapus fakta berdasarkan subject + predicate.
type FactForgetTool struct{ workspace string }

func NewFactForgetTool(workspace string) *FactForgetTool { return &FactForgetTool{workspace} }

func (t *FactForgetTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "fact_forget",
		Description: "Hapus fakta dari fact memory berdasarkan subject dan predicate. Gunakan kalau fakta sudah tidak valid.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject":   map[string]any{"type": "string", "description": "Entitas. Misal: 'Ayah'"},
				"predicate": map[string]any{"type": "string", "description": "Relasi yang mau dihapus. Misal: 'pakai'"},
			},
			"required": []string{"subject", "predicate"},
		},
	}
}

func (t *FactForgetTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	subject, _ := inv.ParsedArgs["subject"].(string)
	predicate, _ := inv.ParsedArgs["predicate"].(string)
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(predicate) == "" {
		return Result{ToolName: inv.ToolName, OK: false, Output: "subject dan predicate wajib diisi"}, nil
	}

	lib, err := factmemory.Open(t.workspace)
	if err != nil {
		return Result{ToolName: inv.ToolName, OK: false, Output: fmt.Sprintf("fact memory error: %v", err)}, nil
	}

	deleted := lib.Delete(subject, predicate)
	if !deleted {
		return Result{ToolName: inv.ToolName, OK: true,
			Output: fmt.Sprintf("Fakta '%s %s ...' tidak ditemukan.", subject, predicate)}, nil
	}
	return Result{ToolName: inv.ToolName, OK: true,
		Output: fmt.Sprintf("✓ Fakta '%s %s ...' dihapus dari fact memory.", subject, predicate)}, nil
}
