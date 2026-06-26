// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func selfURL() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_SELF_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:1987"
}

var hc = &http.Client{Timeout: 20 * time.Second}

func getJSON(path string, out any) error {
	resp, err := hc.Get(selfURL() + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	return json.Unmarshal(b, out)
}

func main() {
	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║  FLOWORK TUI — Category Task console  ║")
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Println("ketik 'help' buat daftar perintah, 'quit' keluar.")
	fmt.Printf("server: %s\n\n", selfURL())

	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("flowork> ")
		if !sc.Scan() {
			break
		}
		f := strings.Fields(strings.TrimSpace(sc.Text()))
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "quit", "exit", "q":
			fmt.Println("dadah bro 👋")
			return
		case "help", "h":
			help()
		case "list", "ls":
			cmdList()
		case "run":
			if len(f) < 3 {
				fmt.Println("pakai: run <kategori> <subjek>   (mis. run saham BBCA)")
				continue
			}
			cmdRun(f[1], strings.Join(f[2:], " "))
		case "runs":
			if len(f) < 2 {
				fmt.Println("pakai: runs <kategori>")
				continue
			}
			cmdRuns(f[1])
		case "result", "r":
			if len(f) < 2 {
				fmt.Println("pakai: result <run_id>")
				continue
			}
			cmdResult(f[1])
		default:
			fmt.Println("perintah ga dikenal. ketik 'help'.")
		}
	}
}

func help() {
	fmt.Println(`  list                       — daftar kategori task
  run <kategori> <subjek>    — jalanin task + tonton timeline live
  runs <kategori>            — riwayat run (review/QC)
  result <run_id>            — detail 1 run (timeline + keputusan)
  help · quit`)
}

func cmdList() {
	var d struct {
		Categories []struct {
			ID, Name, Icon string
			Enabled        bool
		} `json:"categories"`
	}
	if err := getJSON("/api/taskflow/categories", &d); err != nil {
		fmt.Println("err:", err)
		return
	}
	for _, c := range d.Categories {
		st := ""
		if !c.Enabled {
			st = " (off)"
		}
		fmt.Printf("  %s  %-10s %s%s\n", c.Icon, c.ID, c.Name, st)
	}
}

type run struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	InputText string `json:"input_text"`
	Summary   string `json:"summary"`
	StartedAt string `json:"started_at"`
	Steps     []struct {
		AgentID   string `json:"agent_id"`
		RoleLabel string `json:"role_label"`
		Status    string `json:"status"`
		MS        int64  `json:"ms"`
		Err       string `json:"err"`
	} `json:"steps"`
}

var stIcon = map[string]string{"pending": "⚪", "running": "🔵", "done": "✅", "error": "❌", "interrupted": "⏹"}

func cmdRun(cat, subject string) {
	resp, err := hc.Post(selfURL()+"/api/taskflow/run?category="+cat+"&subject="+subject, "", nil)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	var r struct {
		RunID int64  `json:"run_id"`
		Error string `json:"error"`
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(b, &r)
	if r.Error != "" {
		fmt.Println("gagal:", r.Error)
		return
	}
	fmt.Printf("▶ run #%d (%s %s) — nonton timeline (Ctrl-C buat stop nonton)...\n", r.RunID, cat, subject)
	for {
		var rn run
		if err := getJSON(fmt.Sprintf("/api/taskflow/run-detail?id=%d", r.RunID), &rn); err != nil {
			fmt.Println("err:", err)
			return
		}
		printTimeline(rn)
		if rn.Status != "running" {
			return
		}
		time.Sleep(3 * time.Second)
	}
}

func printTimeline(rn run) {
	fmt.Printf("\r\033[K  run #%d [%s] %s\n", rn.ID, rn.Status, rn.InputText)
	for _, s := range rn.Steps {
		d := ""
		if s.MS > 0 {
			d = fmt.Sprintf("%ds", s.MS/1000)
		} else if s.Status == "running" {
			d = "…"
		}
		warn := ""
		if s.Err != "" {
			warn = " ⚠"
		}
		fmt.Printf("    %s %-18s %-4s %s%s\n", stIcon[s.Status], s.AgentID, d, s.RoleLabel, warn)
	}
	if rn.Status == "done" && rn.Summary != "" {
		fmt.Println("  ── KEPUTUSAN ──")
		fmt.Println(indent(rn.Summary, "  "))
	}
}

func cmdRuns(cat string) {
	var d struct {
		Runs []run `json:"runs"`
	}
	if err := getJSON("/api/taskflow/runs?category="+cat+"&limit=15", &d); err != nil {
		fmt.Println("err:", err)
		return
	}
	for _, r := range d.Runs {
		fmt.Printf("  %s #%-3d %-8s %s\n", stIcon[r.Status], r.ID, r.InputText,
			strings.Replace(r.StartedAt, "T", " ", 1))
	}
}

func cmdResult(id string) {
	var rn run
	if err := getJSON("/api/taskflow/run-detail?id="+id, &rn); err != nil {
		fmt.Println("err:", err)
		return
	}
	if rn.ID == 0 {
		fmt.Println("run ga ketemu")
		return
	}
	printTimeline(rn)
}

func indent(s, pre string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pre + l
	}
	return strings.Join(lines, "\n")
}
