// sysstatus_ext.go — SYSTEM-AWARENESS (NON-frozen seam). Sisipin kondisi PC + WAKTU sekarang ke
// SETIAP chat → agent SADAR: spek/OS/CPU/GPU/temp/load + tanggal-jam (biar tau data lama/baru, +
// kalau panas bisa nyaranin jeda). Switch GUI FLOWORK_SYS_STATUS (default ON).
// Multi-OS: detail /proc cuma di linux; OS lain → minimal (OS+waktu).
package main

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/router"
)

func sysStatusOn() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_SYS_STATUS"))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// ---- static (cache sekali) ----
var (
	sysStaticOnce sync.Once
	sysOSRel      string
	sysCPUModel   string
	sysCPUCores   int
	sysRAMTotalGB float64
)

func loadSysStatic() {
	sysStaticOnce.Do(func() {
		sysOSRel = runtime.GOOS
		if runtime.GOOS == "linux" {
			if b, e := os.ReadFile("/proc/sys/kernel/osrelease"); e == nil {
				sysOSRel = "linux " + strings.TrimSpace(string(b))
			}
			// CPU model + cores
			if f, e := os.Open("/proc/cpuinfo"); e == nil {
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					l := sc.Text()
					if sysCPUModel == "" && strings.HasPrefix(l, "model name") {
						if i := strings.Index(l, ":"); i >= 0 {
							sysCPUModel = strings.TrimSpace(l[i+1:])
						}
					}
					if strings.HasPrefix(l, "processor") {
						sysCPUCores++
					}
				}
				f.Close()
			}
			// RAM total
			if mt := readMeminfoKB("MemTotal"); mt > 0 {
				sysRAMTotalGB = float64(mt) / 1024 / 1024
			}
		}
	})
}

func readMeminfoKB(key string) int64 {
	f, e := os.Open("/proc/meminfo")
	if e != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, key+":") {
			fields := strings.Fields(l)
			if len(fields) >= 2 {
				v, _ := strconv.ParseInt(fields[1], 10, 64)
				return v
			}
		}
	}
	return 0
}

func cpuTempC() float64 {
	b, e := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if e != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return v / 1000.0
}

func loadAvg() string {
	b, e := os.ReadFile("/proc/loadavg")
	if e != nil {
		return ""
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 1 {
		return fields[0]
	}
	return ""
}

// GPU via nvidia-smi (cache 30s — exec agak mahal).
var (
	gpuMu     sync.Mutex
	gpuCache  string
	gpuExpiry time.Time
)

func gpuStatus() string {
	gpuMu.Lock()
	defer gpuMu.Unlock()
	if time.Now().Before(gpuExpiry) {
		return gpuCache
	}
	gpuExpiry = time.Now().Add(30 * time.Second)
	gpuCache = ""
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return ""
	}
	out, e := exec.Command(path, "--query-gpu=name,temperature.gpu,utilization.gpu", "--format=csv,noheader,nounits").Output()
	if e != nil {
		return ""
	}
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	parts := strings.Split(line, ",")
	if len(parts) >= 3 {
		gpuCache = strings.TrimSpace(parts[0]) + " " + strings.TrimSpace(parts[1]) + "°C util " + strings.TrimSpace(parts[2]) + "%"
	}
	return gpuCache
}

// systemStatusText — blok [STATUS_PC] live (dipanggil per-chat).
func systemStatusText() string {
	loadSysStatic()
	now := time.Now()
	utc := now.UTC().Format("2006-01-02 15:04")
	local := now.Format("2006-01-02 15:04 MST")

	var b strings.Builder
	b.WriteString("[STATUS_PC] waktu: " + local + " (UTC " + utc + ")")
	b.WriteString(" | OS: " + sysOSRel)
	if sysCPUModel != "" {
		b.WriteString(" | CPU: " + sysCPUModel)
		if sysCPUCores > 0 {
			b.WriteString(" ×" + strconv.Itoa(sysCPUCores))
		}
		if l := loadAvg(); l != "" {
			b.WriteString(" load " + l)
		}
	}
	if sysRAMTotalGB > 0 {
		avail := float64(readMeminfoKB("MemAvailable")) / 1024 / 1024
		b.WriteString(" | RAM: " + strconv.FormatFloat(sysRAMTotalGB-avail, 'f', 1, 64) + "/" + strconv.FormatFloat(sysRAMTotalGB, 'f', 0, 64) + " GB")
	}
	if g := gpuStatus(); g != "" {
		b.WriteString(" | GPU: " + g)
	}
	if t := cpuTempC(); t > 0 {
		b.WriteString(" | CPU " + strconv.FormatFloat(t, 'f', 0, 64) + "°C")
	}
	b.WriteString("\nCatatan: pengetahuan/data lo relatif ke WAKTU di atas — jangan asumsi cutoff lama, cek kebaruan. Kalau GPU/CPU temp tinggi (>80°C) atau load berat, hindari kerjaan berat barengan / sarankan jeda sebentar biar PC ga overheat.")
	return b.String()
}

// InjectSystemStatus — prepend system message [STATUS_PC] ke req (kalau switch ON & belum ada).
func InjectSystemStatus(req *router.OpenAIRequest) {
	if req == nil || !sysStatusOn() {
		return
	}
	for _, m := range req.Messages {
		if m.Role == "system" && strings.Contains(m.Content, "[STATUS_PC]") {
			return // udah ada (anti-dobel)
		}
	}
	msg := router.OpenAIMessage{Role: "system", Content: systemStatusText()}
	req.Messages = append([]router.OpenAIMessage{msg}, req.Messages...)
}
