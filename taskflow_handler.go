// taskflow_handler.go — FASE 4/5: HTTP API Category Task.
//
// Trigger + CRUD kategori/crew + run history (timeline). Definisi task di
// flowork.db (owner-level), di-edit dari GUI tab "Tasks". Run jalan ASYNC
// (background) + step di-persist live → GUI poll run-detail buat timeline.
//
//	POST /api/taskflow/run?category=saham&subject=BBCA  → start run (async), balik run_id
//	     ?solo=1                                          → baseline A/B (sync, 1 agent)
//	GET  /api/taskflow/categories                        → list kategori
//	GET  /api/taskflow/category?id=saham                 → 1 kategori + crew
//	POST /api/taskflow/category                          → upsert kategori + crew (JSON)
//	POST /api/taskflow/category/delete?id=saham          → hapus kategori
//	GET  /api/taskflow/runs?category=saham[&limit=N]     → run history
//	GET  /api/taskflow/run-detail?id=123                 → 1 run + steps (timeline)

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/taskflow"
)

// notifyTelegram — kirim teks ke chat Telegram pakai bot token Mr.Flow (dibaca
// dari secrets state.db-nya). Best-effort: gagal = silent (cuma log). Dipakai
// Fase 6c buat ngirim hasil task balik ke chat yang men-trigger.
// notifyTelegram — kirim hasil task ke chat Telegram (token Mr.Flow). LOGGED di
// tiap titik gagal — anti GHOSTING silent (kalau ga nyampe, ketauan di log,
// bukan diem-diem ilang).
func notifyTelegram(host *kernelhost.Host, chatID, text string) {
	if strings.TrimSpace(chatID) == "" {
		log.Printf("[notify] SKIP — chat_id kosong (task ga di-trigger dari Telegram?)")
		return
	}
	store, err := host.OpenAgentStore("mr-flow")
	if err != nil {
		log.Printf("[notify] GAGAL buka store mr-flow: %v", err)
		return
	}
	defer store.Close()
	secrets, err := store.Secrets()
	if err != nil {
		log.Printf("[notify] GAGAL baca secrets: %v", err)
		return
	}
	token := strings.TrimSpace(secrets["TELEGRAM_BOT_TOKEN"])
	if token == "" {
		log.Printf("[notify] GAGAL — TELEGRAM_BOT_TOKEN kosong di mr-flow")
		return
	}
	if len(text) > 4000 {
		text = text[:4000] + "\n…(dipotong)"
	}
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	req, _ := http.NewRequest(http.MethodPost,
		"https://api.telegram.org/bot"+token+"/sendMessage",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Flowork-Agent/1.0")
	resp, derr := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if derr != nil {
		log.Printf("[notify] GAGAL kirim ke Telegram (chat=%s): %v", chatID, derr)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == 200 {
		log.Printf("[notify] ✓ TERKIRIM ke chat %s (telegram ok)", chatID)
	} else {
		log.Printf("[notify] DITOLAK telegram (chat=%s, http=%d): %s", chatID, resp.StatusCode, string(body))
	}
}

// dbRecorder — implement taskflow.Recorder, persist step ke flowork.db (timeline).
type dbRecorder struct {
	store *floworkdb.Store
	runID int64
}

func (r *dbRecorder) StartStep(agentID, role string, idx int) int64 {
	id, _ := r.store.StartStep(r.runID, agentID, role, idx)
	return id
}
func (r *dbRecorder) FinishStep(stepID int64, status, outputRef, errStr string, ms int64) {
	_ = r.store.FinishStep(stepID, status, outputRef, errStr, ms)
}

func tfWriteJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	if code != 0 {
		w.WriteHeader(code)
	}
	_ = json.NewEncoder(w).Encode(body)
}

// toTaskflowCategory — map floworkdb.TaskCategory → taskflow.Category.
func toTaskflowCategory(c *floworkdb.TaskCategory) taskflow.Category {
	tc := taskflow.Category{ID: c.ID, Name: c.Name, Synthesizer: c.Synthesizer, SynthDirective: c.SynthDirective}
	for _, a := range c.Crew {
		tc.Crew = append(tc.Crew, taskflow.CrewMember{AgentID: a.AgentID, RoleLabel: a.RoleLabel})
	}
	return tc
}

// taskflowRunHandler — POST trigger. Normal = async (timeline). solo = sync (A/B).
func taskflowRunHandler(host *kernelhost.Host, store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		subject := strings.TrimSpace(r.URL.Query().Get("subject"))
		if category == "" || subject == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "category + subject required"})
			return
		}

		// ?solo=1 → BASELINE A/B (sync): 1 agent (analis pertama) ngerjain semua.
		if r.URL.Query().Get("solo") == "1" {
			cat, _ := store.GetCategory(category)
			agentID := "saham-fundamental"
			if cat != nil && len(cat.Crew) > 0 {
				agentID = cat.Crew[0].AgentID
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
			defer cancel()
			reply, ms := taskflow.RunSolo(ctx, host, agentID, subject)
			tfWriteJSON(w, 0, map[string]any{"mode": "solo", "agent": agentID, "ms": ms, "reply": reply})
			return
		}

		notify := strings.TrimSpace(r.URL.Query().Get("notify")) // chat_id Telegram (opsional)
		runID, err := startTaskflowRun(host, store, category, subject, notify)
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{
			"run_id": runID, "status": "running",
			"poll": "/api/taskflow/run-detail?id=" + strconv.FormatInt(runID, 10),
		})
	}
}

// startTaskflowRun — bikin run + jalanin Category Task ASYNC (goroutine) +
// notify Telegram pas kelar. Reusable: dipake HTTP handler + scheduler ticker.
// Balik run_id cepet (run jalan di belakang). Error = validasi gagal.
func startTaskflowRun(host *kernelhost.Host, store *floworkdb.Store, category, subject, notify string) (int64, error) {
	cat, err := store.GetCategory(category)
	if err != nil {
		return 0, err
	}
	if cat == nil {
		return 0, fmt.Errorf("kategori ga ada: %s", category)
	}
	if len(cat.Crew) == 0 {
		return 0, fmt.Errorf("crew kosong — tambah analis dulu")
	}
	runID, err := store.CreateRun(category, subject, "owner", notify)
	if err != nil {
		return 0, fmt.Errorf("create run: %w", err)
	}
	tfCat := toTaskflowCategory(cat)
	catName := cat.Name
	go func() {
		// recover: panic di task (worker/synth) JANGAN crash seluruh binary —
		// tandain run error + log. (Section scanner: bare_goroutine_auditor.)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[taskflow] run #%d PANIC: %v", runID, r)
				_ = store.FinishRun(runID, "error", fmt.Sprintf("panic: %v", r))
			}
		}()
		// 30 menit: crew bisa sampe 6 agent × cap 300s/agent (kernelhost). Budget
		// total mesti muat worst-case, walau rata-rata agent ~120s. Cap, bukan wait.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		rec := &dbRecorder{store: store, runID: runID}
		res := taskflow.RunCategoryTask(ctx, host, host.SharedDir, tfCat, subject, strconv.FormatInt(runID, 10), rec)
		status := "done"
		summary := res.Recommendation
		if res.Err != "" {
			status = "error"
			if summary == "" {
				summary = res.Err
			}
		}
		_ = store.FinishRun(runID, status, summary)
		notifTo := notify
		if notifTo == "" {
			notifTo = "NONE"
		}
		log.Printf("[taskflow] run #%d %s — notify=%s", runID, status, notifTo)
		if notify != "" {
			head := fmt.Sprintf("✅ Hasil %s — %s (run #%d):\n\n", catName, subject, runID)
			if status == "error" {
				head = fmt.Sprintf("⚠️ %s — %s (run #%d) gagal:\n\n", catName, subject, runID)
			}
			notifyTelegram(host, notify, head+summary)
		}
	}()
	return runID, nil
}

// ── Scheduler (looping recurring task) ───────────────────────────────────────

// taskflowSchedulesHandler — GET list jadwal.
func taskflowSchedulesHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListSchedules()
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"schedules": list})
	}
}

// taskflowScheduleAddHandler — POST bikin jadwal (JSON body).
func taskflowScheduleAddHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var sc floworkdb.TaskSchedule
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&sc); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		sc.Category = strings.TrimSpace(sc.Category)
		sc.Subject = strings.TrimSpace(sc.Subject)
		if sc.Category == "" || sc.Subject == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "category + subject required"})
			return
		}
		id, err := store.AddSchedule(sc)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "id": id})
	}
}

// taskflowScheduleDeleteHandler — POST hapus jadwal.
func taskflowScheduleDeleteHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if id <= 0 {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
			return
		}
		if r.URL.Query().Get("enabled") != "" {
			_ = store.ToggleSchedule(id, r.URL.Query().Get("enabled") == "1")
		} else {
			_ = store.DeleteSchedule(id)
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true})
	}
}

// RunDueSchedules — dipanggil ticker tiap menit: fire jadwal yang udah waktunya.
func RunDueSchedules(host *kernelhost.Host, store *floworkdb.Store) int {
	now := time.Now()
	due, err := store.DueSchedules(now)
	if err != nil {
		return 0
	}
	fired := 0
	for _, sc := range due {
		if _, err := startTaskflowRun(host, store, sc.Category, sc.Subject, sc.NotifyChat); err == nil {
			fired++
		}
		_ = store.MarkScheduleFired(sc, now) // tetep advance next_run walau gagal (anti spam)
	}
	return fired
}

// taskflowCategoriesHandler — GET list kategori.
func taskflowCategoriesHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cats, err := store.ListCategories()
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"categories": cats})
	}
}

// taskflowCategoryHandler — GET (detail+crew) / POST (upsert+crew).
func taskflowCategoryHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			id := strings.TrimSpace(r.URL.Query().Get("id"))
			if id == "" {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
				return
			}
			cat, err := store.GetCategory(id)
			if err != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			if cat == nil {
				tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": "ga ada"})
				return
			}
			tfWriteJSON(w, 0, cat)
		case http.MethodPost:
			var body floworkdb.TaskCategory
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
				return
			}
			body.ID = strings.TrimSpace(body.ID)
			if body.ID == "" {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
				return
			}
			if err := store.UpsertCategory(body); err != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			if err := store.SetCrew(body.ID, body.Crew); err != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			tfWriteJSON(w, 0, map[string]any{"ok": true, "id": body.ID})
		default:
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET/POST only"})
		}
	}
}

// taskflowCategoryDeleteHandler — POST hapus kategori.
func taskflowCategoryDeleteHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
			return
		}
		if err := store.DeleteCategory(id); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true})
	}
}

// taskflowRunsHandler — GET run history 1 kategori.
func taskflowRunsHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		if category == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "category required"})
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		runs, err := store.ListRuns(category, limit)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"runs": runs})
	}
}

// taskflowRunDetailHandler — GET 1 run + steps (timeline).
func taskflowRunDetailHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if id <= 0 {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
			return
		}
		run, err := store.GetRun(id)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if run == nil {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": "run ga ada"})
			return
		}
		tfWriteJSON(w, 0, run)
	}
}
