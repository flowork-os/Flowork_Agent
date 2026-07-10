// feature_sysstats_ext.go — STATS SISTEM buat HUD Mr.Flow (owner 2026-07-10). NON-FROZEN sibling.
//
// GET /api/system/stats → {cpu_pct, mem_pct, mem_used_mb, mem_total_mb, load1, uptime_s,
//   router:{ok, version, uptime_s}} — bahan widget HUD (CPU/RAM/load host + status ROUTER :2402).
// CPU% dihitung dari delta /proc/stat antar-panggilan (sampel pertama = 0). Linux-first;
// OS lain balikin field null (graceful, multi-OS aman). Router base: env FLOW_ROUTER_BASE
// (default http://127.0.0.1:2402) — no-hardcode. Session-gated (tidak didaftar public).
// Hapus file ini → fitur mati, koloni utuh.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	sysStart    = time.Now()
	cpuMu       sync.Mutex
	cpuPrevIdle uint64
	cpuPrevTot  uint64
)

func readProcStatCPU() (idle, total uint64, ok bool) {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	f := strings.Fields(line)
	if len(f) < 8 || f[0] != "cpu" {
		return 0, 0, false
	}
	var vals []uint64
	for _, s := range f[1:] {
		v, _ := strconv.ParseUint(s, 10, 64)
		vals = append(vals, v)
		total += v
	}
	if len(vals) >= 5 {
		idle = vals[3] + vals[4] // idle + iowait
	}
	return idle, total, true
}

func cpuPercent() *float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()
	idle, total, ok := readProcStatCPU()
	if !ok {
		return nil
	}
	var pct float64
	if cpuPrevTot != 0 && total > cpuPrevTot {
		dTot := float64(total - cpuPrevTot)
		dIdle := float64(idle - cpuPrevIdle)
		pct = (1 - dIdle/dTot) * 100
		if pct < 0 {
			pct = 0
		}
	}
	cpuPrevIdle, cpuPrevTot = idle, total
	return &pct
}

func memInfo() (usedMB, totalMB *int64, pct *float64) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, nil, nil
	}
	var tot, avail int64
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(f[1], 10, 64) // kB
		switch f[0] {
		case "MemTotal:":
			tot = v
		case "MemAvailable:":
			avail = v
		}
	}
	if tot == 0 {
		return nil, nil, nil
	}
	u := (tot - avail) / 1024
	t := tot / 1024
	p := float64(tot-avail) / float64(tot) * 100
	return &u, &t, &p
}

func loadAvg1() *float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	f := strings.Fields(string(raw))
	if len(f) < 1 {
		return nil
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return nil
	}
	return &v
}

func routerBase() string {
	if b := os.Getenv("FLOW_ROUTER_BASE"); b != "" {
		return strings.TrimRight(b, "/")
	}
	return "http://127.0.0.1:2402"
}

func routerStatus() map[string]any {
	cl := &http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Get(routerBase() + "/api/health")
	if err != nil {
		return map[string]any{"ok": false}
	}
	defer resp.Body.Close()
	var j map[string]any
	if json.NewDecoder(resp.Body).Decode(&j) != nil || resp.StatusCode != 200 {
		return map[string]any{"ok": false}
	}
	return map[string]any{"ok": true, "version": j["version"], "uptime_s": j["uptime"]}
}

func sysStatsHandler(w http.ResponseWriter, r *http.Request) {
	usedMB, totalMB, memPct := memInfo()
	out := map[string]any{
		"cpu_pct":      cpuPercent(),
		"mem_pct":      memPct,
		"mem_used_mb":  usedMB,
		"mem_total_mb": totalMB,
		"load1":        loadAvg1(),
		"uptime_s":     int64(time.Since(sysStart).Seconds()),
		"router":       routerStatus(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func init() {
	RegisterFeature(Feature{Name: "sys-stats", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/system/stats", sysStatsHandler)
	}})
}
