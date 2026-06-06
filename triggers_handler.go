// triggers_handler.go — HTTP untuk ROADMAP 3 (Trigger). Manajemen aturan + tipe + run +
// webhook intake. Semua (kecuali /hook/) owner-loopback + session (di-wrap authMgr.Middleware
// di main.go). /hook/ = intake push (mesin-ke-mesin), secret-gated per-rule.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/triggers"
)

var triggerIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)

func randSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GET /api/triggers (list) · POST /api/triggers (create/update).
func triggersHandler(eng *triggers.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list, err := eng.Store.ListTriggers()
			if err != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			tfWriteJSON(w, 0, map[string]any{"triggers": list, "count": len(list)})
		case http.MethodPost:
			var b struct {
				ID, Name, TypeID, Config, Target, TargetKind, Prompt, Deliver string
				Enabled                                                       bool
			}
			// terima snake_case juga
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
			var m map[string]any
			if json.Unmarshal(raw, &m) != nil {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
				return
			}
			gs := func(k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
			b.ID, b.Name, b.TypeID = gs("id"), gs("name"), gs("type_id")
			b.Config, b.Target, b.TargetKind = gs("config"), gs("target"), gs("target_kind")
			b.Prompt, b.Deliver = gs("prompt"), gs("deliver")
			if en, ok := m["enabled"].(bool); ok {
				b.Enabled = en
			} else {
				b.Enabled = true
			}
			if !triggerIDRe.MatchString(b.ID) {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id harus slug ^[a-z0-9][a-z0-9-]{1,40}$"})
				return
			}
			typ, ok := triggers.GetType(b.TypeID)
			if !ok {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "type_id tak dikenal: " + b.TypeID})
				return
			}
			if b.Name == "" || b.Target == "" || b.Prompt == "" {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "name/target/prompt wajib"})
				return
			}
			if b.Config == "" {
				b.Config = "{}"
			}
			if b.TargetKind != "group" {
				b.TargetKind = "agent"
			}
			if b.Deliver == "" {
				b.Deliver = "telegram"
			}
			// webhook → pastikan punya secret (generate sekali, simpan).
			secret := ""
			if typ.Mode() == "webhook" {
				if cur, _ := eng.Store.GetTrigger(b.ID); cur == nil || cur.WebhookSecret == "" {
					secret = randSecret()
				}
			}
			if err := eng.Store.UpsertTrigger(floworkdb.Trigger{
				ID: b.ID, Name: b.Name, TypeID: b.TypeID, Config: b.Config, Target: b.Target,
				TargetKind: b.TargetKind, Prompt: b.Prompt, Deliver: b.Deliver, Enabled: b.Enabled,
				WebhookSecret: secret,
			}); err != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			tfWriteJSON(w, 0, map[string]any{"ok": true, "id": b.ID})
		default:
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method"})
		}
	}
}

func triggersDeleteHandler(eng *triggers.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !triggerIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		if err := eng.Store.DeleteTrigger(id); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "deleted": id})
	}
}

func triggersToggleHandler(eng *triggers.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !triggerIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		on := r.URL.Query().Get("enabled") == "1" || strings.EqualFold(r.URL.Query().Get("enabled"), "true")
		if err := eng.Store.SetTriggerEnabled(id, on); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "enabled": on})
	}
}

func triggersRunHandler(eng *triggers.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !triggerIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		runID, err := eng.RunNow(id)
		if err != nil {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "run_id": runID})
	}
}

func triggersRunsHandler(eng *triggers.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !triggerIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		limit := 50
		if n, e := strconv.Atoi(r.URL.Query().Get("limit")); e == nil && n > 0 {
			limit = n
		}
		runs, err := eng.Store.ListTriggerRuns(id, limit)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"runs": runs, "count": len(runs)})
	}
}

// GET /api/triggers/types — daftar TIPE terdaftar (untuk GUI form). Plug-and-play: tipe baru
// (file type_*.go self-register) otomatis muncul di sini tanpa ubah handler.
func triggersTypesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		out := []map[string]any{}
		for _, t := range triggers.ListTypes() {
			out = append(out, map[string]any{
				"id": t.ID(), "name": t.Name(), "mode": t.Mode(),
				"config_schema": t.ConfigSchema(), "payload_keys": t.PayloadKeys(),
			})
		}
		tfWriteJSON(w, 0, map[string]any{"types": out})
	}
}

// POST /api/triggers/hook/<id>?key=<secret> — INTAKE webhook (push). EXEMPT session (lihat
// floworkauth public-path); diamankan per-rule secret (constant-time di engine).
func triggersHookHandler(eng *triggers.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/triggers/hook/")
		if !triggerIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err := eng.HandleWebhook(id, r.URL.Query().Get("key"), body); err != nil {
			tfWriteJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true})
	}
}
