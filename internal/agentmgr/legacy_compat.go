// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Shim adapter — map reference GUI tab paths (dari
//   Pictures/stable_open_router/.../guiapi/static/tabs/) ke endpoint
//   agent-scoped kita. Default agent = mr-flow (single-warga). Phase 2
//   (multi-agent agent_id selector di GUI header) → tambah file baru.
//
// legacy_compat.go — reference GUI tab compat shim.

package agentmgr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/protector"
	"flowork-gui/internal/wallet"
)

// defaultAgentID — single-warga shim. Phase 2: read dari X-Agent-ID header.
const defaultAgentID = "mr-flow"

// =============================================================================
// /api/wallet (reference: wallet.js)
// =============================================================================

// WalletCompatHandler — GET /api/wallet
// Reference shape: {configured, wallet, total_usd, holdings[], partial_error}
func WalletCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"configured": false, "error": err.Error()})
		return
	}
	defer store.Close()
	addrs, err := store.ListWalletAddresses()
	if err != nil || len(addrs) == 0 {
		httpx.WriteJSON(w, map[string]any{"configured": false})
		return
	}
	// Use first address.
	addr := addrs[0]
	portfolio, perr := wallet.Snapshot(r.Context(), addr.Address)
	if perr != nil {
		httpx.WriteJSON(w, map[string]any{
			"configured":    true,
			"wallet":        addr.Address,
			"total_usd":     0,
			"holdings":      []any{},
			"partial_error": perr.Error(),
		})
		return
	}
	// Save snapshot best-effort.
	body, _ := json.Marshal(portfolio)
	_, _ = store.InsertWalletSnapshot(portfolio.TotalUSD, string(body))

	httpx.WriteJSON(w, map[string]any{
		"configured":    true,
		"wallet":        addr.Address,
		"total_usd":     portfolio.TotalUSD,
		"holdings":      portfolio.Holdings,
		"partial_error": portfolio.PartialErr,
	})
}

// WalletTxCompatHandler — GET /api/wallet/tx?limit=10
// Reference shape: {tx: [{hash, direction, amount, symbol, ts}]}.
// Kita ngga ada tx — adapter dari wallet_snapshots.
func WalletTxCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	limit := 10
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"tx": []any{}})
		return
	}
	defer store.Close()
	snaps, err := store.ListWalletSnapshots(limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"tx": []any{}})
		return
	}
	tx := make([]map[string]any, 0, len(snaps))
	for _, s := range snaps {
		tx = append(tx, map[string]any{
			"hash":      fmt.Sprintf("snapshot-%d", s.ID),
			"direction": "snapshot",
			"amount":    s.TotalUSD,
			"symbol":    "USD",
			"ts":        s.TakenAt,
		})
	}
	httpx.WriteJSON(w, map[string]any{"tx": tx, "count": len(tx)})
}

// =============================================================================
// /api/finance/snapshot (reference: finance.js)
// =============================================================================

// FinanceSnapshotCompatHandler — GET /api/finance/snapshot
// Reference shape: {total_usd, by_category, daily, budgets, recent_calls}
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
	// Today summary.
	summary, _ := store.SummaryLedger(todayStart(), todayEnd())
	var total float64
	for _, s := range summary {
		total += s.CostUSD
	}
	budgets, _ := store.ListBudgets()
	recent, _ := store.ListLedger("", "", "", 10)
	httpx.WriteJSON(w, map[string]any{
		"total_usd":    total,
		"by_category":  summary,
		"budgets":      budgets,
		"recent_calls": recent,
	})
}

// =============================================================================
// /api/brain/prompt-templates (reference: prompt.js)
// =============================================================================

// PromptTemplatesListCompatHandler — GET /api/brain/prompt-templates
// Reference shape: {templates: [{name, description, ...}]}
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
		templates = append(templates, map[string]any{
			"name":        s.Slot,
			"description": fmt.Sprintf("v%d · %d bytes · %s", s.Version, len(s.Body), s.Notes),
			"version":     s.Version,
		})
	}
	httpx.WriteJSON(w, map[string]any{"templates": templates, "count": len(templates)})
}

// PromptTemplatesDetailCompatHandler — GET /api/brain/prompt-templates/detail?name=
// Reference shape: {name, body, description}
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
		"name":        sp.Slot,
		"body":        sp.Body,
		"description": sp.Notes,
		"version":     sp.Version,
	})
}

// PromptTemplatesUpsertCompatHandler — POST /api/brain/prompt-templates
// + POST /api/brain/prompt-templates/update (same handler — upsert via UNIQUE).
func PromptTemplatesUpsertCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Name        string `json:"name"`
		Body        string `json:"body"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	if body.Name == "" || body.Body == "" {
		httpx.WriteJSON(w, map[string]any{"error": "name + body required"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	id, err := store.SetSelfPrompt(body.Name, body.Body, body.Description, 0)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
}

// PromptTemplatesDeleteCompatHandler — POST /api/brain/prompt-templates/delete
// Soft-delete: insert empty body v+1. Phase 2 add real delete.
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
	// Soft-delete: store empty body sebagai sentinel.
	_, _ = store.SetSelfPrompt(body.Name, "[deleted]", "soft-delete", 0)
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

// =============================================================================
// Helpers
// =============================================================================

func todayStart() string {
	return timeFmt(0)
}
func todayEnd() string {
	return timeFmt(86400)
}
func timeFmt(deltaSec int64) string {
	t := time.Now().UTC()
	if deltaSec > 0 {
		t = t.Add(time.Duration(deltaSec) * time.Second)
	}
	return t.Format("2006-01-02") + "T00:00:00Z"
}

// =============================================================================
// /api/protector (reference: protector.js)
// =============================================================================

// ProtectorListCompatHandler — GET /api/protector
// Reference shape: {rules: [{id, path, type, action, enabled, source}]}
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
	custom, _ := store.ListProtectorRules()
	out := []map[string]any{}
	for _, b := range protector.Baseline() {
		out = append(out, map[string]any{
			"id":      0,
			"path":    b.Pattern,
			"type":    b.Type,
			"action":  b.Action,
			"enabled": true,
			"source":  "hardcoded",
		})
	}
	for _, c := range custom {
		out = append(out, map[string]any{
			"id":      c.ID,
			"path":    c.Pattern,
			"type":    c.RuleType,
			"action":  c.Action,
			"enabled": c.Enabled,
			"source":  c.Source,
		})
	}
	httpx.WriteJSON(w, map[string]any{"rules": out, "count": len(out)})
}

// ProtectorAddCompatHandler — POST /api/protector/add
func ProtectorAddCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Path   string `json:"path"`
		Type   string `json:"type"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if body.Type == "" {
		body.Type = "file_path"
	}
	if body.Action == "" {
		body.Action = "block"
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	id, err := store.AddProtectorRule(agentdb.ProtectorRule{
		RuleType: body.Type, Pattern: body.Path, Action: body.Action,
		Source: "custom", Enabled: true,
	})
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
}

// ProtectorRemoveCompatHandler — POST /api/protector/remove {id}
func ProtectorRemoveCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		ID int64 `json:"id"`
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
	if err := store.DeleteProtectorRule(body.ID); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

// ProtectorToggleCompatHandler — POST /api/protector/toggle {id, enabled}
func ProtectorToggleCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		ID      int64 `json:"id"`
		Enabled bool  `json:"enabled"`
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
	if err := store.ToggleProtectorRule(body.ID, body.Enabled); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

// ProtectorTestCompatHandler — GET /api/protector/test?path=
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
		"path":    path,
		"matched": hit,
		"pattern": matched.Pattern,
		"action":  matched.Action,
	})
}

// =============================================================================
// /api/codemap (reference: codemap.js)
// =============================================================================

// CodemapGraphCompatHandler — GET /api/codemap/graph
// Reference shape: {nodes: [{id, name, path, type}], edges: []}
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
	rows, err := store.ListCodemapNodes("", "", "", 500)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	nodes := make([]map[string]any, 0, len(rows))
	for _, n := range rows {
		nodes = append(nodes, map[string]any{
			"id":         n.ID,
			"name":       n.Name,
			"path":       n.FilePath,
			"type":       n.NodeType,
			"layer":      n.Layer,
			"line_start": n.LineStart,
			"line_end":   n.LineEnd,
			"size_loc":   n.SizeLOC,
		})
	}
	httpx.WriteJSON(w, map[string]any{
		"nodes": nodes,
		"edges": []any{}, // phase 2: Section 27 phase 2 edges
		"count": len(nodes),
	})
}

// CodemapStatusCompatHandler — GET /api/codemap/status
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
	all, _ := store.ListCodemapNodes("", "", "", 5000)
	byType := map[string]int{}
	for _, n := range all {
		byType[n.NodeType]++
	}
	httpx.WriteJSON(w, map[string]any{
		"indexed":  true,
		"total":    len(all),
		"by_type":  byType,
	})
}

// CodemapZombiesCompatHandler — GET /api/codemap/zombies
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
	rows, err := store.ListZombieFindings(200)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	zombies := make([]map[string]any, 0, len(rows))
	for _, z := range rows {
		zombies = append(zombies, map[string]any{
			"id":           z.ID,
			"path":         z.FilePath,
			"name":         z.SymbolName,
			"type":         z.SymbolType,
			"confidence":   z.Confidence,
			"reason":       z.Reason,
			"acknowledged": z.Acknowledged,
		})
	}
	httpx.WriteJSON(w, map[string]any{"zombies": zombies, "count": len(zombies)})
}

// CodemapReindexCompatHandler — POST /api/codemap/reindex (no-op stub).
// Real reindex membutuhkan iterasi semua file workspace + parse + upsert.
// Phase 2 implementation.
func CodemapReindexCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":      true,
		"message": "reindex stub — gunakan POST /api/agents/codemap/index untuk index file specific",
	})
}

// CodemapRootsCompatHandler — GET /api/codemap/roots (return file_path list).
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

// keep agentdb import used.
var _ = agentdb.Decision{}
