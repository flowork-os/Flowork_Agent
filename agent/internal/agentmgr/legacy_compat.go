// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/codemap"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/protector"
)

func codemapRoot() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_CODEMAP_ROOT")); v != "" {
		return v
	}
	return agentdb.ProjectRoot()
}

const defaultAgentID = "mr-flow"

func FinanceSnapshotCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7).Format(time.RFC3339)
	to := now.Format(time.RFC3339)

	summary, _ := store.SummaryLedger(from, to)
	var total float64
	for _, s := range summary {
		total += s.CostUSD
	}

	rawBudgets, _ := store.ListBudgets()
	budgets := make([]map[string]any, 0, len(rawBudgets))
	for _, b := range rawBudgets {
		pct := 0.0
		if b.BudgetValue > 0 {
			pct = total / b.BudgetValue * 100
		}
		budgets = append(budgets, map[string]any{
			"metric_key":     b.MetricKey,
			"budget_value":   b.BudgetValue,
			"warning_at_pct": b.WarningAtPct,
			"enabled":        b.Enabled,
			"spent_usd":      total,
			"pct":            pct,
		})
	}
	recent, _ := store.ListLedger("", "", "", 15)

	httpx.WriteJSON(w, map[string]any{
		"api_cost_total_usd":   total,
		"api_cost_by_category": summary,
		"budgets":              budgets,
		"recent_calls":         recent,
		"updated_at":           to,
	})
}

func PromptTemplatesListCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"templates": []any{}})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	slots, err := store.ListSelfPromptSlots()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	templates := make([]map[string]any, 0, len(slots))
	for _, s := range slots {
		if s.Body == promptDeletedSentinel {
			continue
		}
		preview := s.Body
		if len(preview) > 300 {
			preview = preview[:300]
		}
		templates = append(templates, map[string]any{
			"name":         s.Slot,
			"preview":      preview,
			"content_size": len(s.Body),
			"updated_at":   s.UpdatedAt,
			"usage_count":  1,
		})
	}
	httpx.WriteJSON(w, map[string]any{"templates": templates, "count": len(templates)})
}

const promptDeletedSentinel = "[deleted]"

func PromptTemplatesDetailCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		httpx.WriteJSON(w, map[string]any{"error": "name required"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	sp, err := store.GetSelfPrompt(name, 0)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"name":       sp.Slot,
		"content":    sp.Body,
		"used_by":    []map[string]any{{"name": defaultAgentID}},
		"used_count": 1,
		"updated_at": sp.UpdatedAt,
	})
}

func PromptTemplatesUpsertCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Name    string `json:"name"`
		Content string `json:"content"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	content := body.Content
	if content == "" {
		content = body.Body
	}
	if body.Name == "" || content == "" {
		httpx.WriteJSON(w, map[string]any{"error": "name + content required"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	id, err := store.SetSelfPrompt(body.Name, content, "", 0)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
}

func PromptTemplatesDeleteCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	_, _ = store.SetSelfPrompt(body.Name, promptDeletedSentinel, "soft-delete", 0)
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

func ProtectorListCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	custom, _ := store.ListProtectorRulesCat()
	out := []map[string]any{}

	for _, b := range protector.Baseline() {
		out = append(out, map[string]any{
			"path":     b.Pattern,
			"category": b.Type,
			"action":   b.Action,
			"active":   true,
			"source":   "hardcoded",
		})
	}

	for _, c := range custom {
		cat := c.Category
		if cat == "" {
			cat = c.RuleType
		}
		out = append(out, map[string]any{
			"path":     c.Pattern,
			"category": cat,
			"action":   c.Action,
			"active":   c.Enabled,
			"source":   c.Source,
		})
	}
	httpx.WriteJSON(w, map[string]any{"rules": out, "count": len(out)})
}

func ProtectorAddCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Path     string `json:"path"`
		Type     string `json:"type"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		httpx.WriteJSON(w, map[string]any{"error": "path required"})
		return
	}
	if body.Category == "" {
		body.Category = "custom"
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	id, err := store.AddProtectorRuleCat("file_path", body.Path, "block", body.Category)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
}

func ProtectorRemoveCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	id, ok := store.FindProtectorRuleIDByPattern(body.Path)
	if !ok {
		httpx.WriteJSON(w, map[string]any{"error": "rule not found (hardcoded baseline tidak bisa dihapus)"})
		return
	}
	if err := store.DeleteProtectorRule(id); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

func ProtectorToggleCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Path   string `json:"path"`
		Active bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	id, ok := store.FindProtectorRuleIDByPattern(body.Path)
	if !ok {
		httpx.WriteJSON(w, map[string]any{"error": "rule not found (hardcoded baseline selalu aktif)"})
		return
	}
	if err := store.ToggleProtectorRule(id, body.Active); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

func ProtectorTestCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		httpx.WriteJSON(w, map[string]any{"error": "path required"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	customRows, _ := store.ListProtectorRules()
	customRules := []protector.BaselineRule{}
	for _, c := range customRows {
		if c.Enabled {
			customRules = append(customRules, protector.BaselineRule{
				Type: c.RuleType, Pattern: c.Pattern, Action: c.Action,
			})
		}
	}
	matched, hit := protector.CheckPattern("file_path", path, customRules)
	httpx.WriteJSON(w, map[string]any{
		"path":      path,
		"protected": hit,
		"matched":   hit,
		"pattern":   matched.Pattern,
		"action":    matched.Action,
	})
}

func CodemapGraphCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	nodes, err := store.ListCodemapFiles()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	edges, _ := store.ListCodemapFileEdges()
	httpx.WriteJSON(w, map[string]any{
		"nodes":      nodes,
		"edges":      edges,
		"node_count": len(nodes),
		"edge_count": len(edges),
	})
}

func CodemapStatusCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	nodes, _ := store.ListCodemapFiles()
	edges, _ := store.ListCodemapFileEdges()
	httpx.WriteJSON(w, map[string]any{
		"running":    false,
		"node_count": len(nodes),
		"edge_count": len(edges),
	})
}

func CodemapZombiesCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	files, err := store.ListCodemapFiles()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	edges, _ := store.ListCodemapFileEdges()
	dirOf := func(p string) string {
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			return p[:i]
		}
		return ""
	}
	baseOf := func(p string) string {
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			return p[i+1:]
		}
		return p
	}
	hasOutgoing := map[string]bool{}
	dirImported := map[string]bool{}
	for _, e := range edges {
		hasOutgoing[e.From] = true
		if dirOf(e.From) != dirOf(e.To) {
			dirImported[dirOf(e.To)] = true
		}
	}
	dirHasMain := map[string]bool{}
	for _, f := range files {
		if p, _ := f["path"].(string); baseOf(p) == "main.go" {
			dirHasMain[dirOf(p)] = true
		}
	}
	skipFile := func(p string) bool {
		b := baseOf(p)
		if b == "main.go" || strings.HasSuffix(b, "_test.go") {
			return true
		}
		for _, suf := range []string{"_linux.go", "_darwin.go", "_windows.go", "_other.go", "_wasm.go", "_js.go", "_unix.go", "_amd64.go", "_arm64.go"} {
			if strings.HasSuffix(b, suf) {
				return true
			}
		}
		return false
	}
	zombies := []map[string]any{}
	for _, f := range files {
		path, _ := f["path"].(string)
		dep, _ := f["dependent_count"].(int)
		if dep == 0 && !hasOutgoing[path] && !dirImported[dirOf(path)] && !dirHasMain[dirOf(path)] && !skipFile(path) {
			zombies = append(zombies, map[string]any{
				"path":       path,
				"name":       f["name"],
				"file_type":  f["file_type"],
				"line_count": f["line_count"],
			})
		}
	}
	httpx.WriteJSON(w, map[string]any{
		"zombies": zombies, "count": len(zombies),
		"advisory": true,
		"note":     "HEURISTIK LEMAH (import Go = level-paket): KANDIDAT review manual, JANGAN auto-delete. Verifikasi pakai grep/go build dulu.",
	})
}

func CodemapReindexCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	files, edges, err := codemap.WalkRepo(codemapRoot())
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	rows := make([]agentdb.CodemapFile, 0, len(files))
	for _, f := range files {
		rows = append(rows, agentdb.CodemapFile{
			Path: f.Path, Name: f.Name, FileType: f.FileType, LineCount: f.LineCount,
			Layer: f.Layer, HasTests: f.HasTests, HasDocs: f.HasDocs,
			HealthScore: f.HealthScore, RecentlyTouched: f.RecentlyTouched, Issues: f.Issues,
		})
	}
	eRows := make([]agentdb.CodemapFileEdge, 0, len(edges))
	for _, e := range edges {
		eRows = append(eRows, agentdb.CodemapFileEdge{From: e.From, To: e.To})
	}
	if err := store.ReplaceCodemapFiles(rows, eRows); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}

	root := codemapRoot()
	nodeCount := 0
	for _, f := range files {
		if f.FileType != "go" {
			continue
		}
		content, rerr := os.ReadFile(filepath.Join(root, f.Path))
		if rerr != nil {
			continue
		}
		ns, perr := codemap.ParseGo(f.Path, content)
		if perr != nil {
			continue
		}
		_ = store.DeleteCodemapNodesByFile(f.Path)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, n := range ns {
			if _, e := store.UpsertCodemapNode(agentdb.CodemapNode{
				NodeType: n.Type, Name: n.Name, FilePath: f.Path,
				LineStart: n.LineStart, LineEnd: n.LineEnd, Layer: f.Layer,
				Signature: n.Signature, SizeLOC: n.SizeLOC,
				LastModified: now, IndexedAt: now,
			}); e == nil {
				nodeCount++
			}
		}
	}

	httpx.WriteJSON(w, map[string]any{
		"ok":         true,
		"message":    "reindex selesai (file + node level)",
		"file_count": len(rows),
		"node_count": nodeCount,
		"edge_count": len(eRows),
	})
}

func CodemapDocsCompatHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	root := codemapRoot()
	clean := filepath.Clean(filepath.Join(root, rel))

	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		http.Error(w, "path escapes root", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	const maxDocBytes = 256 * 1024
	if len(data) > maxDocBytes {
		data = data[:maxDocBytes]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	lang := strings.TrimPrefix(filepath.Ext(rel), ".")
	_, _ = w.Write([]byte("```" + lang + "\n"))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n```\n"))
}

func CodemapRootsCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	all, _ := store.ListCodemapNodes("", "", "", 1000)
	seen := map[string]bool{}
	roots := []string{}
	for _, n := range all {
		if !seen[n.FilePath] {
			seen[n.FilePath] = true
			roots = append(roots, n.FilePath)
		}
	}
	httpx.WriteJSON(w, map[string]any{"roots": roots, "count": len(roots)})
}

var _ = agentdb.Decision{}
