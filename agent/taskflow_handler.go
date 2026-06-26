// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/loket"
	"flowork-gui/internal/taskflow"
)

func notifyTelegram(host *kernelhost.Host, chatID, text string) {
	if strings.TrimSpace(chatID) == "" {
		log.Printf("[notify] SKIP — chat_id kosong (task ga di-trigger dari Telegram?)")
		return
	}

	token := ""
	if store, err := host.OpenAgentStore("mr-flow"); err == nil {
		if secrets, serr := store.Secrets(); serr == nil {
			token = strings.TrimSpace(secrets["TELEGRAM_BOT_TOKEN"])
		}
		store.Close()
	}

	if token == "" {
		if store, err := host.OpenAgentStore("telegram-channel"); err == nil {
			if secrets, serr := store.Secrets(); serr == nil {
				token = strings.TrimSpace(secrets["TELEGRAM_BOT_TOKEN"])
			}
			store.Close()
		}
	}
	if token == "" {
		if fdb, ferr := floworkdb.Shared(); ferr == nil {
			if v, _ := fdb.GetSecret("TELEGRAM_BOT_TOKEN"); strings.TrimSpace(v) != "" {
				token = strings.TrimSpace(v)
			}
		}
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("FLOWORK_TG_BOT_TOKEN"))
	}
	if token == "" {
		log.Printf("[notify] SKIP — TELEGRAM_BOT_TOKEN kosong (telegram-channel store + env)")
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

func toTaskflowCategory(c *floworkdb.TaskCategory) taskflow.Category {
	tc := taskflow.Category{ID: c.ID, Name: c.Name, Synthesizer: c.Synthesizer, SynthDirective: c.SynthDirective, WorkerDirective: c.WorkerDirective}
	for _, a := range c.Crew {
		tc.Crew = append(tc.Crew, taskflow.CrewMember{AgentID: a.AgentID, RoleLabel: a.RoleLabel})
	}
	return tc
}

func taskflowRunHandler(host *kernelhost.Host, store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		group := strings.TrimSpace(r.URL.Query().Get("group"))
		subject := strings.TrimSpace(r.URL.Query().Get("subject"))
		if subject == "" || (category == "" && group == "") {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "subject + (category atau group) wajib"})
			return
		}

		if group != "" {
			notify := strings.TrimSpace(r.URL.Query().Get("notify"))
			runID, err := startGroupTaskRun(host, store, group, subject, notify)
			if err != nil {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			tfWriteJSON(w, 0, map[string]any{"run_id": runID, "status": "running", "group": group,
				"poll": "/api/taskflow/run-detail?id=" + strconv.FormatInt(runID, 10)})
			return
		}

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

		notify := strings.TrimSpace(r.URL.Query().Get("notify"))
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

var groupIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,39}$`)

func startGroupTaskRun(host *kernelhost.Host, store *floworkdb.Store, groupID, subject, notify string) (int64, error) {
	if !groupIDRe.MatchString(groupID) {
		return 0, fmt.Errorf("group id invalid (^[a-z0-9][a-z0-9-]{1,39}$): %q", groupID)
	}

	staged := filepath.Join(host.AgentsDir, groupID+".fwagent")
	loketPath := filepath.Join(filepath.Dir(agentdb.Resolve(groupID, staged)), "loket.db")
	st, err := loket.OpenStore(loketPath)
	if err != nil {
		return 0, fmt.Errorf("buka loket group %q: %w", groupID, err)
	}
	isGroup, _, _ := st.KVGet("group")
	membersCSV, _, _ := st.KVGet("members")
	st.Close()
	if strings.TrimSpace(isGroup) != "1" {
		return 0, fmt.Errorf("%q bukan group (group!=1)", groupID)
	}
	hasMember := false
	for _, m := range strings.Split(membersCSV, ",") {
		if strings.TrimSpace(m) != "" {
			hasMember = true
			break
		}
	}
	if !hasMember {
		return 0, fmt.Errorf("group %q gak ada member", groupID)
	}
	runID, err := store.CreateRun(groupID, subject, "owner", notify)
	if err != nil {
		return 0, fmt.Errorf("create run: %w", err)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[taskflow] group run #%d PANIC: %v", runID, r)
				_ = store.FinishRun(runID, "error", fmt.Sprintf("panic: %v", r))
			}
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reply, ierr := host.InvokeAgentMessageTimeout(ctx, groupID, subject, "telegram-async", 25*time.Minute)
		status, summary := "done", strings.TrimSpace(reply)
		if ierr != nil {
			status = "error"
			summary = ierr.Error()
		} else if summary == "" {
			status = "error"
			summary = "group balik kosong"
		}
		_ = store.FinishRun(runID, status, summary)
		if notify != "" {
			head := fmt.Sprintf("✅ Hasil tim %s — %s (run #%d):\n\n", groupID, subject, runID)
			if status == "error" {
				head = fmt.Sprintf("⚠️ tim %s — %s (run #%d) gagal:\n\n", groupID, subject, runID)
			}
			notifyTelegram(host, notify, head+summary)
		} else {
			log.Printf("[taskflow] group run #%d %s — notify=NONE", runID, status)
		}
	}()
	return runID, nil
}

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

	live := make(map[string]bool)
	for _, id := range host.AgentIDs() {
		live[id] = true
	}
	anyLive := false
	for _, m := range cat.Crew {
		if live[m.AgentID] {
			anyLive = true
			break
		}
	}
	if !anyLive {
		return 0, fmt.Errorf("crew '%s' udah ga ada — semua %d member-nya udah dihapus; bikin ulang crew-nya dulu", cat.Name, len(cat.Crew))
	}
	runID, err := store.CreateRun(category, subject, "owner", notify)
	if err != nil {
		return 0, fmt.Errorf("create run: %w", err)
	}
	tfCat := toTaskflowCategory(cat)
	catName := cat.Name
	go func() {

		defer func() {
			if r := recover(); r != nil {
				log.Printf("[taskflow] run #%d PANIC: %v", runID, r)
				_ = store.FinishRun(runID, "error", fmt.Sprintf("panic: %v", r))
			}
		}()

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
		_ = store.MarkScheduleFired(sc, now)
	}
	return fired
}

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
