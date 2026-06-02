// taskflow_handler.go — FASE 4: HTTP trigger buat Category Task orchestrator.
//
// POST /api/taskflow/run?category=saham&subject=BBCA
//   → jalanin crew (fan-out) + synthesizer (fan-in) → balikin keputusan + steps.
// GET  /api/taskflow/categories  → list kategori kedaftar.
//
// Ini trigger buat TEST + nanti dipanggil Mr.Flow (via tool task_run) / scheduler.
// Long-running (banyak LLM call) → timeout gede.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/taskflow"
)

var taskflowRunSeq uint64

func taskflowRunHandler(host *kernelhost.Host) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "POST only"})
			return
		}
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		subject := strings.TrimSpace(r.URL.Query().Get("subject"))
		if category == "" || subject == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "category + subject required"})
			return
		}
		seq := atomic.AddUint64(&taskflowRunSeq, 1)
		runID := strconv.FormatInt(time.Now().Unix(), 10) + "-" + strconv.FormatUint(seq, 10)

		// Long-running: 3 analis + synthesizer, tiap-tiap tool-loop LLM. Kasih
		// budget waktu gede; tiap InvokeAgentMessage punya timeout 180s sendiri.
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		// ?solo=1 → BASELINE A/B: 1 agent (saham-fundamental) ngerjain semua sendiri.
		if r.URL.Query().Get("solo") == "1" {
			reply, ms := taskflow.RunSolo(ctx, host, "saham-fundamental", subject)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode": "solo", "agent": "saham-fundamental", "subject": subject,
				"ms": ms, "reply": reply,
			})
			return
		}
		res := taskflow.RunCategoryTask(ctx, host, host.SharedDir, category, subject, runID)
		_ = json.NewEncoder(w).Encode(res)
	}
}

func taskflowCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"categories": taskflow.Categories()})
}
