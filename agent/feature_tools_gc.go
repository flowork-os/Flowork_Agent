// 🔒 FROZEN tools-core — Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// JANGAN edit tanpa unfreeze SADAR owner (di-hash KERNEL_FREEZE.md + chattr +i). Baca lock/tools.md DULU.
// SWITCH ambang GC = ENV (FLOWORK_TOOL_GC_MAXERR/_IDLE_DAYS/_OFF) — atur lewat env, JANGAN edit file ini.
//
// feature_tools_gc.go — GC + DELETION-AWARE buat sidecar tools (owner 2026-06-23).
//
// SELEKSI ALAM: tool yg sering ERROR (rusak, mis. API berubah) / NGANGGUR lama (obsolete) → auto-HAPUS.
// DELETION-AWARE (MATANG): pas tool mati, bersihin sampai OTAK — quarantine cognitive-node `agent:<id>/
// tool/<name>` + turunin confidence instinct yg nyebut tool itu. TOMBSTONE-based: tiap sweep quarantine
// ULANG tool-node yg mati (nutup celah dream re-project tool-hantu dari pengalaman lama). main.go frozen
// GA disentuh (file feature baru). Blueprint: roadmap §15.4/§15.7.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/toolsidecar"
)

func init() {
	RegisterFeature(Feature{Name: "tools-gc", Phase: PhaseSeed, Apply: func(d *Deps) {
		// Endpoint: trigger GC manual + status (GET/POST sama).
		d.Mux.HandleFunc("/api/tools/gc", func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, runToolGC(d.Host))
		})
		// Ticker otonom: GC tiap 6 jam (seleksi alam jalan sendiri). Matiin: FLOWORK_TOOL_GC_OFF=1.
		if strings.TrimSpace(os.Getenv("FLOWORK_TOOL_GC_OFF")) == "1" {
			return
		}
		go func() {
			t := time.NewTicker(6 * time.Hour)
			defer t.Stop()
			for range t.C {
				res := runToolGC(d.Host)
				if dl, _ := res["deleted"].([]map[string]any); len(dl) > 0 {
					log.Printf("tools-gc: %d tool di-prune (seleksi alam)", len(dl))
				}
			}
		}()
	}})
}

func gcEnvInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
func gcMaxErr() int   { return gcEnvInt("FLOWORK_TOOL_GC_MAXERR", 5) }     // error >= N → prune
func gcIdleDays() int { return gcEnvInt("FLOWORK_TOOL_GC_IDLE_DAYS", 90) } // nganggur > N hari → prune

// runToolGC — scan → prune tool error-tinggi/nganggur → quarantine cognition (deletion-aware).
func runToolGC(host *kernelhost.Host) map[string]any {
	deleted := []map[string]any{}
	for _, dec := range toolsidecar.GCScan(gcMaxErr(), gcIdleDays()) {
		owner, _, err := toolsidecar.DeleteTool(toolsidecar.ToolsDir(), dec.Name)
		if err != nil {
			continue
		}
		log.Printf("tools-gc: HAPUS tool %q (owner=%s) — %s", dec.Name, owner, dec.Reason)
		deleted = append(deleted, map[string]any{"name": dec.Name, "owner": owner, "reason": dec.Reason})
	}
	// DELETION-AWARE: quarantine cognition tool-mati (incl re-project dream) di SEMUA agent.
	quarantined := tombstoneSweep(host)
	return map[string]any{
		"deleted":               deleted,
		"cognition_quarantined": quarantined,
		"maxErr":                gcMaxErr(),
		"idleDays":              gcIdleDays(),
		"note":                  "tool error-tinggi/nganggur di-prune; cognition tool-mati di-quarantine (agent ga halu tool-hantu).",
	}
}

// tombstoneSweep — tiap agent: quarantine cognitive-node `agent:<id>/tool/<name>` buat tiap tool yg udah
// MATI (tombstone) + turunin confidence instinct yg nyebut. Idempoten — re-quarantine kalau dream
// re-project. Pakai store.DB() (UpsertNode ga update status; ini jalur bersih via host store).
func tombstoneSweep(host *kernelhost.Host) int {
	if host == nil {
		return 0
	}
	toms := toolsidecar.Tombstones(toolsidecar.ToolsDir())
	if len(toms) == 0 {
		return 0
	}
	n := 0
	for _, id := range host.AgentIDs() {
		store, err := host.OpenAgentStore(id)
		if err != nil {
			continue
		}
		db := store.DB()
		for _, name := range toms {
			nodeID := "agent:" + id + "/tool/" + name
			if res, e := db.Exec(`UPDATE cognitive_nodes SET status='quarantined' WHERE id=? AND status!='quarantined'`, nodeID); e == nil {
				if c, _ := res.RowsAffected(); c > 0 {
					n += int(c)
				}
			}
			// Instinct yg nyebut tool mati → turunin confidence (residue dream, anti "WHEN..→ pake tool X" mati).
			// Floor 0.05: decay geometris (×0.3) konvergen lalu BERHENTI nulis — ga ngegerus instinct mati
			// terus-terusan tiap 6 jam (idempoten-praktis). LIKE '% name%'/'%name%' heuristik linkage.
			_, _ = db.Exec(`UPDATE cognitive_nodes SET confidence=confidence*0.3
				WHERE type='instinct' AND status='active' AND confidence>0.05 AND label LIKE ?`, "%"+name+"%")
		}
		store.Close()
	}
	return n
}
