// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package localai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/sidecar"
)

type Runtime struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	modelName string
	port      int
	binPath   string
	cli       *http.Client
}

func NewRuntime(binPath string, port int) *Runtime {
	if binPath == "" {
		if binPath = ResolveLlamaBin(); binPath == "" {
			binPath = "llama-server"
		}
	}
	if port <= 0 {
		port = 8088
	}
	return &Runtime{
		binPath: binPath,
		port:    port,
		cli:     &http.Client{Timeout: 10 * time.Second},
	}
}

const FloworkBrainModel = "flowork-brain"

func ResolveFloworkBrain() string {

	return sidecar.ModelGGUF()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func ResolveLlamaBin() string {

	if p := sidecar.LlamaBin(); p != "" {
		return p
	}
	if lp, err := exec.LookPath("llama-server"); err == nil {
		return lp
	}
	return ""
}

func (r *Runtime) Start(modelName, ggufPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
		_, _ = r.cmd.Process.Wait()
		r.cmd = nil
	}

	if modelName == "" {
		modelName = FloworkBrainModel
	}
	if ggufPath == "" && modelName == FloworkBrainModel {
		ggufPath = ResolveFloworkBrain()
	}
	if modelName == "" || ggufPath == "" {
		return fmt.Errorf("model_name + gguf_path required (flowork-brain.gguf not found under models/ — run from repo root or set $FLOWORK_BRAIN_GGUF)")
	}

	cw := strings.TrimSpace(os.Getenv("FLOWORK_CTX"))
	if cw == "" {
		cw = "16384"
	}
	args := []string{
		"-m", ggufPath,
		"--port", strconv.Itoa(r.port),
		"--host", "127.0.0.1",
		"--jinja",
		"-c", cw,
	}

	reasoning := strings.TrimSpace(os.Getenv("FLOWORK_REASONING"))
	if reasoning == "" {
		reasoning = "off"
	}
	args = append(args, "--reasoning", reasoning)

	args = append(args, "--reasoning-format", "deepseek")

	if ngl := strings.TrimSpace(os.Getenv("FLOWORK_NGL")); ngl != "" {
		args = append(args, "-ngl", ngl)
	}

	if os.Getenv("FLOWORK_CPU_MOE") == "1" {
		args = append(args, "--cpu-moe")
	}

	if kt := strings.TrimSpace(os.Getenv("FLOWORK_KV_TYPE")); kt != "" {
		args = append(args, "-fa", "on", "-ctk", kt, "-ctv", kt)
	}

	if cr := strings.TrimSpace(os.Getenv("FLOWORK_CACHE_REUSE")); cr != "" && cr != "0" && !strings.EqualFold(cr, "off") {
		args = append(args, "--cache-reuse", cr)
	}

	if np := strings.TrimSpace(os.Getenv("FLOWORK_PARALLEL_SLOTS")); np != "" && np != "0" {
		args = append(args, "-np", np)
	}

	if sp := strings.TrimSpace(os.Getenv("FLOWORK_SLOT_SAVE_PATH")); sp != "" {
		args = append(args, "--slot-save-path", sp)
	}
	cmd := exec.Command(r.binPath, args...)

	binDir := filepath.Dir(r.binPath)
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+binDir+string(os.PathListSeparator)+os.Getenv("LD_LIBRARY_PATH"))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start llama-server: %w", err)
	}
	r.cmd = cmd
	r.modelName = modelName

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if r.healthy() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("llama-server start timeout (port %d)", r.port)
}

func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	_ = r.cmd.Process.Kill()
	_, _ = r.cmd.Process.Wait()
	r.cmd = nil
	r.modelName = ""
	return nil
}

type Status struct {
	Running   bool   `json:"running"`
	ModelName string `json:"model_name"`
	Port      int    `json:"port"`
	Healthy   bool   `json:"healthy"`
}

func (r *Runtime) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := Status{
		Port:      r.port,
		ModelName: r.modelName,
	}
	if r.cmd != nil && r.cmd.Process != nil {
		st.Running = true

	}
	if st.Running {
		st.Healthy = r.healthyUnlocked()
	}
	return st
}

func (r *Runtime) healthy() bool {
	return r.healthyUnlocked()
}

func (r *Runtime) healthyUnlocked() bool {
	url := fmt.Sprintf("http://127.0.0.1:%d/health", r.port)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := r.cli.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return strings.Contains(string(body), "ok") || resp.StatusCode == 200
}
