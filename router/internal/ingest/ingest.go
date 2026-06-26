// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package ingest

import (
	"context"
	"fmt"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

type Req struct {
	Content    string  `json:"content"`
	Wing       string  `json:"wing,omitempty"`
	Room       string  `json:"room,omitempty"`
	SourceType string  `json:"source_type,omitempty"`
	SourceFile string  `json:"source_file,omitempty"`
	MemType    string  `json:"mem_type,omitempty"`
	Importance float64 `json:"importance,omitempty"`
	ChunkIndex int     `json:"chunk_index,omitempty"`
}

type Result struct {
	DrawerID string `json:"drawer_id"`
	Added    bool   `json:"added"`
	Note     string `json:"note,omitempty"`
	Error    string `json:"error,omitempty"`
}

func Submit(ctx context.Context, req Req) Result {
	content := Sanitize(req.Content)
	if content == "" {
		return Result{Error: "content empty after sanitize"}
	}
	if minContentChars > 0 && len(content) < minContentChars {
		return Result{Note: fmt.Sprintf("skipped: content < %d chars", minContentChars)}
	}
	importance := req.Importance
	if importance <= 0 {
		importance = Score(content, req.SourceType)
	}

	id, added, err := brain.AddDrawerFull(ctx, brain.AddDrawerOpts{
		Content:    content,
		Wing:       req.Wing,
		Room:       req.Room,
		SourceType: req.SourceType,
		SourceFile: req.SourceFile,
		MemType:    req.MemType,
		Importance: importance,
		ChunkIndex: req.ChunkIndex,
	})
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{DrawerID: id, Added: added}
}

func SubmitBatch(ctx context.Context, items []Req) []Result {
	out := make([]Result, 0, len(items))
	for _, it := range items {
		out = append(out, Submit(ctx, it))
	}
	return out
}

type BatchStats struct {
	Total   int `json:"total"`
	Added   int `json:"added"`
	Deduped int `json:"deduped"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func Summarize(results []Result) BatchStats {
	var s BatchStats
	s.Total = len(results)
	for _, r := range results {
		switch {
		case r.Error != "":
			s.Failed++
		case r.Note != "":
			s.Skipped++
		case r.Added:
			s.Added++
		default:
			s.Deduped++
		}
	}
	return s
}

const minContentChars = 20
