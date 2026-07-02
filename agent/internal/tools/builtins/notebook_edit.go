// notebook_edit.go — SIBLING ext (⚠️ FROZEN 2026-07-02 seizin owner — stabil+live): tool `notebook_edit` buat
// ngedit Jupyter .ipynb PER-CELL (roadmap "buka lock": tool edit .ipynb per-cell).
// Plug-and-play: init() self-register ke papan tools.Register (NOL sentuh builtins.go
// frozen). Hapus file → tool ilang, core utuh. 📄 Dok: lock/notebook-edit.md
//
// Semantik ala Claude NotebookEdit: replace source 1 cell, insert cell baru, atau
// delete cell — target by index atau by cell id (nbformat 4.5+). Isolasi WORKSPACE
// terjaga (resolveFileArgs → workspace-confined, sama kayak file_write).
package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"flowork-gui/internal/tools"
)

func init() { tools.Register(&notebookEditTool{}) }

type notebookEditTool struct{}

func (notebookEditTool) Name() string       { return "notebook_edit" }
func (notebookEditTool) Capability() string { return "fs:write:/shared/*" }

func (notebookEditTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Edit a Jupyter notebook (.ipynb) one cell at a time. edit_mode=replace " +
			"overwrites a cell's source; insert adds a new cell; delete removes one. Target a cell " +
			"by cell_index (0-based) or cell_id. Isolated to your workspace.",
		Params: []tools.Param{
			{Name: "file_path", Type: tools.ParamString, Description: "relative path to the .ipynb inside your workspace, e.g. 'notebooks/analysis.ipynb'", Required: true},
			{Name: "edit_mode", Type: tools.ParamString, Description: "replace | insert | delete (default replace)"},
			{Name: "cell_index", Type: tools.ParamInt, Description: "0-based cell position. replace/delete: which cell. insert: where to insert (default: append at end)."},
			{Name: "cell_id", Type: tools.ParamString, Description: "target cell by its nbformat id (alternative to cell_index) for replace/delete"},
			{Name: "new_source", Type: tools.ParamString, Description: "new cell content (required for replace/insert)"},
			{Name: "cell_type", Type: tools.ParamString, Description: "code | markdown — for insert (default code)"},
		},
		Returns: "{path, edit_mode, cell_index, cells: [{index, type, id, preview}], cell_count}",
	}
}

func (notebookEditTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	abs, rel, err := resolveFileArgs(ctx, args)
	if err != nil {
		return tools.Result{}, err
	}
	if !strings.HasSuffix(strings.ToLower(abs), ".ipynb") {
		return tools.Result{}, fmt.Errorf("notebook_edit: file_path harus berakhiran .ipynb (dapet %q)", rel)
	}

	mode := strings.ToLower(strings.TrimSpace(nbArgString(args, "edit_mode")))
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" && mode != "insert" && mode != "delete" {
		return tools.Result{}, fmt.Errorf("notebook_edit: edit_mode harus replace|insert|delete (dapet %q)", mode)
	}

	raw, rerr := os.ReadFile(abs)
	if rerr != nil {
		return tools.Result{}, fmt.Errorf("notebook_edit: baca %q: %w", rel, rerr)
	}
	// Unmarshal ke map generik → pertahankan SEMUA field nbformat (metadata, outputs,
	// nbformat_minor, dll) apa adanya pas ditulis balik (anti-korup notebook user).
	var nb map[string]any
	if jerr := json.Unmarshal(raw, &nb); jerr != nil {
		return tools.Result{}, fmt.Errorf("notebook_edit: %q bukan JSON .ipynb valid: %w", rel, jerr)
	}
	cellsAny, _ := nb["cells"].([]any)
	if nb["cells"] == nil {
		cellsAny = []any{}
	}

	// Resolusi target cell (index atau id).
	idx64, haveIdx := argInt(args, "cell_index")
	idx := int(idx64)
	cellID := strings.TrimSpace(nbArgString(args, "cell_id"))
	if cellID != "" {
		found := -1
		for i, c := range cellsAny {
			if cm, ok := c.(map[string]any); ok {
				if id, _ := cm["id"].(string); id == cellID {
					found = i
					break
				}
			}
		}
		if found < 0 {
			return tools.Result{}, fmt.Errorf("notebook_edit: cell_id %q ga ketemu", cellID)
		}
		idx, haveIdx = found, true
	}

	newSource := nbArgString(args, "new_source")

	switch mode {
	case "replace":
		if !haveIdx {
			return tools.Result{}, fmt.Errorf("notebook_edit: replace butuh cell_index atau cell_id")
		}
		if idx < 0 || idx >= len(cellsAny) {
			return tools.Result{}, fmt.Errorf("notebook_edit: cell_index %d di luar rentang (0..%d)", idx, len(cellsAny)-1)
		}
		cm, ok := cellsAny[idx].(map[string]any)
		if !ok {
			return tools.Result{}, fmt.Errorf("notebook_edit: cell %d rusak", idx)
		}
		cm["source"] = toNbSource(newSource)
		// Source berubah → output lama basi. Reset (Jupyter semantics) buat code cell.
		if ct, _ := cm["cell_type"].(string); ct == "code" {
			cm["outputs"] = []any{}
			cm["execution_count"] = nil
		}

	case "insert":
		at := len(cellsAny) // default append
		if haveIdx {
			at = idx
		}
		if at < 0 || at > len(cellsAny) {
			return tools.Result{}, fmt.Errorf("notebook_edit: cell_index %d di luar rentang insert (0..%d)", at, len(cellsAny))
		}
		ct := strings.ToLower(strings.TrimSpace(nbArgString(args, "cell_type")))
		if ct == "" {
			ct = "code"
		}
		if ct != "code" && ct != "markdown" {
			return tools.Result{}, fmt.Errorf("notebook_edit: cell_type harus code|markdown (dapet %q)", ct)
		}
		newCell := map[string]any{
			"cell_type": ct,
			"metadata":  map[string]any{},
			"source":    toNbSource(newSource),
		}
		if ct == "code" {
			newCell["outputs"] = []any{}
			newCell["execution_count"] = nil
		}
		cellsAny = append(cellsAny, nil)
		copy(cellsAny[at+1:], cellsAny[at:])
		cellsAny[at] = newCell
		idx = at

	case "delete":
		if !haveIdx {
			return tools.Result{}, fmt.Errorf("notebook_edit: delete butuh cell_index atau cell_id")
		}
		if idx < 0 || idx >= len(cellsAny) {
			return tools.Result{}, fmt.Errorf("notebook_edit: cell_index %d di luar rentang (0..%d)", idx, len(cellsAny)-1)
		}
		cellsAny = append(cellsAny[:idx], cellsAny[idx+1:]...)
	}

	nb["cells"] = cellsAny
	out, merr := json.MarshalIndent(nb, "", " ")
	if merr != nil {
		return tools.Result{}, fmt.Errorf("notebook_edit: encode: %w", merr)
	}
	out = append(out, '\n')
	if werr := os.WriteFile(abs, out, 0o644); werr != nil {
		return tools.Result{}, fmt.Errorf("notebook_edit: tulis %q: %w", rel, werr)
	}

	return tools.Result{
		Output: map[string]any{
			"path":       rel,
			"edit_mode":  mode,
			"cell_index": idx,
			"cell_count": len(cellsAny),
			"cells":      summarizeCells(cellsAny),
		},
		Note: fmt.Sprintf("notebook %s: %s cell (skarang %d cell)", rel, mode, len(cellsAny)),
	}, nil
}

// toNbSource — string → format source nbformat (list baris, tiap baris bawa '\n'
// kecuali terakhir). Kosong → list kosong.
func toNbSource(s string) []any {
	if s == "" {
		return []any{}
	}
	parts := strings.SplitAfter(s, "\n")
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue // buntut kosong dari SplitAfter pas string diakhiri '\n'
		}
		out = append(out, p)
	}
	return out
}

// summarizeCells — ringkasan tiap cell (index/type/id/preview) buat model tau state baru.
func summarizeCells(cells []any) []map[string]any {
	out := make([]map[string]any, 0, len(cells))
	for i, c := range cells {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		ct, _ := cm["cell_type"].(string)
		id, _ := cm["id"].(string)
		preview := nbSourceToString(cm["source"])
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		preview = strings.ReplaceAll(preview, "\n", " ")
		out = append(out, map[string]any{"index": i, "type": ct, "id": id, "preview": preview})
	}
	return out
}

// nbArgString — ambil arg string (kosong kalau ga ada / bukan string).
func nbArgString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

// nbSourceToString — source nbformat (list baris ATAU string) → string utuh.
func nbSourceToString(src any) string {
	switch v := src.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, l := range v {
			if s, ok := l.(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	}
	return ""
}
