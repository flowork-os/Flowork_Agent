// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/zombie"
)

func ZombieFindingsHandler(w http.ResponseWriter, r *http.Request) {
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	switch r.Method {
	case http.MethodGet:
		rows, err := store.ListZombieFindings(parseLimitOr(r.URL.Query().Get("limit"), 100))
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
	case http.MethodPost:
		var body agentdb.ZombieFinding
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		id, err := store.AddZombieFinding(body)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

func ZombieScanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	minAge := 30
	if s := r.URL.Query().Get("min_age_days"); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n >= 0 {
			minAge = n
		}
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	sharedRoot := agentFolder(agentID) + "/workspace"
	res, err := zombie.Scan(r.Context(), store, zombie.ScanOptions{
		SharedRoot: sharedRoot,
		MinAgeDays: minAge,
	})
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":               true,
		"symbols_scanned":  res.SymbolsScanned,
		"files_grepped":    res.FilesGrepped,
		"inserted":         res.Inserted,
		"findings_preview": res.Findings,
	})
}

func ZombieAckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	findingID, _ := strconv.ParseInt(r.URL.Query().Get("finding_id"), 10, 64)
	if agentID == "" || findingID == 0 {
		httpx.WriteJSON(w, map[string]any{"error": "id + finding_id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if err := store.AcknowledgeZombie(findingID); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

func SelfPromptRenderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
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
	slotOrder := []string{"system", "persona", "guideline", "task"}
	byName := map[string]agentdb.SelfPrompt{}
	for _, s := range slots {
		byName[s.Slot] = s
	}
	var b strings.Builder

	b.WriteString(WIBNowHeader())

	b.WriteString(RecoveryDoctrine())
	emitted := []string{}
	emitOne := func(slot string) {
		if sp, ok := byName[slot]; ok {
			b.WriteString("# ")
			b.WriteString(strings.ToUpper(slot))
			b.WriteString(" (v")
			b.WriteString(strconv.Itoa(sp.Version))
			b.WriteString(")\n")
			b.WriteString(sp.Body)
			b.WriteString("\n\n")
			emitted = append(emitted, slot)
			delete(byName, slot)
		}
	}
	for _, s := range slotOrder {
		emitOne(s)
	}
	leftKeys := make([]string, 0, len(byName))
	for k := range byName {
		leftKeys = append(leftKeys, k)
	}
	for i := 0; i < len(leftKeys); i++ {
		for j := i + 1; j < len(leftKeys); j++ {
			if leftKeys[j] < leftKeys[i] {
				leftKeys[i], leftKeys[j] = leftKeys[j], leftKeys[i]
			}
		}
	}
	for _, s := range leftKeys {
		emitOne(s)
	}
	httpx.WriteJSON(w, map[string]any{
		"rendered":     b.String(),
		"slots_used":   emitted,
		"length_bytes": b.Len(),
	})
}

func SelfPromptHandler(w http.ResponseWriter, r *http.Request) {
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	switch r.Method {
	case http.MethodGet:
		slot := strings.TrimSpace(r.URL.Query().Get("slot"))
		if slot == "" {
			rows, err := store.ListSelfPromptSlots()
			if err != nil {
				httpx.WriteJSON(w, map[string]any{"error": err.Error()})
				return
			}
			httpx.WriteJSON(w, map[string]any{"slots": rows, "count": len(rows)})
			return
		}
		version, _ := strconv.Atoi(r.URL.Query().Get("version"))
		sp, err := store.GetSelfPrompt(slot, version)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, sp)
	case http.MethodPost:
		var body agentdb.SelfPrompt
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		id, err := store.SetSelfPrompt(body.Slot, body.Body, body.Notes, body.Version)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}
