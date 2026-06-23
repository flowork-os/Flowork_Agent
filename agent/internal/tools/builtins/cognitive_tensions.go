// 🔒 FROZEN COGNITIVE-GRAPH · Repo: https://github.com/flowork-os/Flowork-OS · Owner: Aola Sahidin (Mr.Dev)
// ⛔ WAJIB sebelum ngedit: BACA /home/mrflow/Documents/FLowork_os/lock/CognitiveGraph.md.
//    File BEKU (chattr +i + hash). Tool CGM baru → FILE BARU cognitive_<nama>.go + init() sendiri
//    (akses store via tools.FromStore, panggil method Store yg ADA). JANGAN buka file beku ini.
//
// cognitive_tensions.go — tool buat mr-flow (atau agent manapun) SADAR + RESOLVE kontradiksi
// data di cognitive graph-nya sendiri, lewat KLARIFIKASI ke owner.
//
// Konteks (owner 2026-06-23): cognitive graph nyimpen "open contradictions" (tension) = relasi
// FUNGSIONAL yg konflik (mis. owner goal_is X dulu, sekarang Y). Sebelum ini cuma nampang PASIF
// di GUI tab Cognitive Graph. Sekarang mr-flow bisa LIHAT (cognitive_tensions) + setelah owner
// mutusin, RESOLVE (cognitive_resolve) → data makin akurat dari obrolan.
//
// Cara nambah tool TANPA bongkar brain-core frozen: file baru + init() sendiri (Go gabung
// semua init() sepaket). Akses store agent langsung via tools.FromStore(ctx); method
// ListOpenTensions/UpsertEdge/ResolveTension udah ada di cognitive_gate.go/cognitive_graph.go
// (frozen, cukup DIPANGGIL). Arsitektur graph → lihat lock/brain.md.
package builtins

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&cognitiveTensionsTool{})
	tools.Register(&cognitiveResolveTool{})
}

// argIntTension — ambil int dari args (JSON kirim number sbg float64; toleran string/int).
func argIntTension(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// ── cognitive_tensions — LIHAT daftar kontradiksi yg nunggu keputusan owner ──
type cognitiveTensionsTool struct{}

func (cognitiveTensionsTool) Name() string       { return "cognitive_tensions" }
func (cognitiveTensionsTool) Capability() string { return "state:read" }
func (cognitiveTensionsTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Daftar KONTRADIKSI data (open contradictions) di cognitive graph kamu yg NUNGGU keputusan owner. " +
			"Tiap item: id, subject, relation, nilai LAMA vs BARU yg konflik (mis. owner goal_is X dulu, sekarang Y). " +
			"PAKAI ini kalau ragu fakta/preferensi owner, atau mau bantu beresin data. Hasilnya buat KLARIFIKASI ke owner, " +
			"JANGAN ditebak sendiri — owner yang decide, lalu pakai cognitive_resolve.",
		Params: []tools.Param{
			{Name: "limit", Type: tools.ParamInt, Description: "max item (default 20, max 200)"},
		},
		Returns: "{tensions:[{id, subject, relation, old, new, detail}], count}",
	}
}
func (cognitiveTensionsTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	items, err := store.ListOpenTensions(argIntTension(args, "limit", 20))
	if err != nil {
		return tools.Result{}, fmt.Errorf("list tensions: %w", err)
	}
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		out = append(out, map[string]any{
			"id": t.ID, "subject": t.FromID, "relation": t.RelationType,
			"old": t.OldToID, "new": t.NewToID, "detail": t.Detail,
		})
	}
	return tools.Result{Output: map[string]any{"tensions": out, "count": len(out)}}, nil
}

// ── cognitive_resolve — RESOLVE 1 kontradiksi SETELAH owner mutusin ──
type cognitiveResolveTool struct{}

func (cognitiveResolveTool) Name() string       { return "cognitive_resolve" }
func (cognitiveResolveTool) Capability() string { return "state:write" }
func (cognitiveResolveTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Resolve 1 kontradiksi (tension) SETELAH owner mutusin yg bener. keep='new' (pakai nilai BARU → " +
			"di-apply jadi edge aktif, yg lama di-superseded) atau keep='old' (pertahanin nilai LAMA). Lalu tension ditutup. " +
			"⛔ WAJIB owner yang mutusin — JANGAN nebak. Cek dulu cognitive_tensions buat id-nya.",
		Params: []tools.Param{
			{Name: "tension_id", Type: tools.ParamInt, Description: "id tension (dari cognitive_tensions)", Required: true},
			{Name: "keep", Type: tools.ParamString, Description: "'new' (pakai nilai baru) atau 'old' (pertahanin lama)", Required: true},
		},
		Returns: "{ok, resolved, kept, now}",
	}
}
func (cognitiveResolveTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	id := int64(argIntTension(args, "tension_id", 0))
	if id == 0 {
		return tools.Result{}, fmt.Errorf("tension_id wajib")
	}
	keep := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["keep"])))
	if keep != "new" && keep != "old" {
		return tools.Result{}, fmt.Errorf("keep harus 'new' atau 'old'")
	}
	// Cari tension by id (ga ada GetByID — list lalu filter; open tension <= 200).
	items, err := store.ListOpenTensions(200)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list: %w", err)
	}
	var t *agentdb.CogTension
	for i := range items {
		if items[i].ID == id {
			t = &items[i]
			break
		}
	}
	if t == nil {
		return tools.Result{}, fmt.Errorf("tension #%d ga ketemu (mungkin udah resolved)", id)
	}
	applied := t.OldToID
	if keep == "new" {
		// New edge sengaja DITAHAN pas konflik (cognitive_dream) → apply sekarang + superseded yg lama.
		if err := store.UpsertEdge(agentdb.CogEdge{
			FromID: t.FromID, RelationType: t.RelationType, ToID: t.NewToID,
			Status: "active", SourceKind: "owner_decided", Confidence: 1.0,
		}); err != nil {
			return tools.Result{}, fmt.Errorf("apply edge baru: %w", err)
		}
		_ = store.UpsertEdge(agentdb.CogEdge{
			FromID: t.FromID, RelationType: t.RelationType, ToID: t.OldToID,
			Status: "superseded", SourceKind: "owner_decided",
		})
		applied = t.NewToID
	}
	// keep=='old': old masih aktif, new ga pernah di-apply → cukup tutup tension.
	if err := store.ResolveTension(id); err != nil {
		return tools.Result{}, fmt.Errorf("resolve: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"ok": true, "resolved": id, "kept": keep,
		"now": fmt.Sprintf("%s -[%s]-> %s", t.FromID, t.RelationType, applied),
	}}, nil
}
