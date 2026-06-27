// cliadapter — CLI-Adapter Core GENERIK (ROADMAP_REPO_TO_APP F1). Satu binary dikirim sekali:
// dia yang ngomong protokol core Flowork (stdio baris-JSON, lihat internal/apps/proc.go) lalu
// nerjemahin tiap `op` jadi panggilan command repo yang dipetakan di `adapter.json`.
//
// Tujuan: repo mentah (yt-dlp, script python, CLI node, biner) GA usah ngerti protokol Flowork —
// adapter ini jembatannya. Engine `runtime:process` udah jalanin command apa aja sbg core, jadi
// adapter = "core generik" → NOL ubah engine (seam, bukan bongkar inti).
//
// WHITE-LABEL: nol identitas corporate ke-hardcode. Multi-OS: pure-Go, no shell (exec argv), CGO-off.
package cliadapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ConfigName — file peta op→command, ada di folder app (cwd adapter).
const ConfigName = "adapter.json"

// defaultTimeout — batas 1 op kalau adapter.json ga nentuin.
const defaultTimeout = 120 * time.Second

// OpSpec — satu operasi: command + cara nyuntik args.
//
// Placeholder {key} di argv selalu disubstitusi dari args. arg_style nentuin sisa args:
//   - "placeholder" (default): cuma substitusi {key}, sisanya diabaikan.
//   - "flags": tiap key yang BUKAN placeholder di-append sbg "--key value".
//   - "args_list": elemen array args["args"] di-append apa adanya (bungkus CLI: yt-dlp <url> -f mp4).
//   - "json_stdin": seluruh args di-marshal JSON → dikirim ke stdin command.
//   - "none": args diabaikan total.
type OpSpec struct {
	Cmd        []string `json:"cmd"`         // argv; elemen[0] = program, sisanya argumen (boleh {key})
	ArgStyle   string   `json:"arg_style"`   // placeholder|flags|json_stdin|none
	Workdir    string   `json:"workdir"`     // override workdir global utk op ini (relatif folder app)
	TimeoutSec int      `json:"timeout_sec"` // override timeout utk op ini
}

// Config — isi adapter.json.
type Config struct {
	Workdir    string            `json:"workdir"`     // subdir tempat command jalan (default "."), relatif folder app
	TimeoutSec int               `json:"timeout_sec"` // default semua op
	Env        map[string]string `json:"env"`         // env tambahan (brand/white-label dari sini, BUKAN hardcode)
	Ops        map[string]OpSpec `json:"ops"`
}

// request/response — protokol stdio (identik proc.go ↔ coreResp di apps.go).
type request struct {
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args"`
}

// LoadConfig — baca adapter.json dari folder app.
func LoadConfig(baseDir string) (*Config, error) {
	raw, err := os.ReadFile(filepath.Join(baseDir, ConfigName))
	if err != nil {
		return nil, fmt.Errorf("baca %s: %w", ConfigName, err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ConfigName, err)
	}
	if len(c.Ops) == 0 {
		return nil, errors.New(ConfigName + " tak punya 'ops'")
	}
	return &c, nil
}

// Run — loop utama core: baca {"op","args"}\n dari in, balas {"result",...}\n / {"error"}\n ke out.
// baseDir = folder app (cwd, tempat adapter.json + kode repo). Berhenti saat in EOF.
func Run(in io.Reader, out io.Writer, baseDir string) error {
	cfg, err := LoadConfig(baseDir)
	if err != nil {
		// adapter.json rusak = fatal config → lapor 1x via stderr, exit non-nol (lewat caller).
		return err
	}
	r := bufio.NewReaderSize(in, 1<<20)
	w := bufio.NewWriter(out)
	defer w.Flush()

	var stateVer int64
	for {
		line, rerr := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			stateVer = handleLine(w, baseDir, cfg, line, stateVer)
			_ = w.Flush()
		}
		if rerr != nil {
			return nil // EOF / stream tutup = selesai normal
		}
	}
}

// handleLine — proses 1 baris request, tulis 1 baris response. Balik stateVer baru.
func handleLine(w io.Writer, baseDir string, cfg *Config, line []byte, stateVer int64) int64 {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		writeError(w, "request bukan JSON valid: "+err.Error())
		return stateVer
	}
	spec, ok := cfg.Ops[req.Op]
	if !ok {
		writeError(w, "op tak terdaftar di "+ConfigName+": "+req.Op)
		return stateVer
	}
	args := map[string]any{}
	if len(req.Args) > 0 {
		_ = json.Unmarshal(req.Args, &args) // args bebas; gagal-parse = args kosong
	}
	result, err := execOp(baseDir, cfg, spec, args)
	if err != nil {
		writeError(w, err.Error())
		return stateVer
	}
	stateVer++
	writeResult(w, result, stateVer)
	return stateVer
}

// execOp — rakit argv (substitusi placeholder + arg_style) → exec di workdir → tangkap stdout/stderr/exit.
func execOp(baseDir string, cfg *Config, spec OpSpec, args map[string]any) (map[string]any, error) {
	if len(spec.Cmd) == 0 || strings.TrimSpace(spec.Cmd[0]) == "" {
		return nil, errors.New("op tak punya 'cmd'")
	}

	used := map[string]bool{}
	argv := make([]string, len(spec.Cmd))
	for i, part := range spec.Cmd {
		argv[i] = substitute(part, args, used)
	}

	var stdinData []byte
	switch strings.ToLower(spec.ArgStyle) {
	case "flags":
		for k, v := range args {
			if used[k] {
				continue
			}
			argv = append(argv, "--"+k, toStr(v))
		}
	case "args_list":
		if lst, ok := args["args"].([]any); ok {
			for _, e := range lst {
				argv = append(argv, toStr(e))
			}
		}
	case "json_stdin":
		stdinData, _ = json.Marshal(args)
	case "none", "placeholder", "":
		// placeholder-only (default) / none: ga ada tambahan
	default:
		return nil, errors.New("arg_style tak dikenal: " + spec.ArgStyle)
	}

	// workdir: op override > config > folder app. Containment anti-traversal keluar folder app.
	wd := firstNonEmpty(spec.Workdir, cfg.Workdir, ".")
	full := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(wd)))
	if rel, err := filepath.Rel(baseDir, full); err != nil || strings.HasPrefix(rel, "..") {
		return nil, errors.New("workdir di luar folder app (anti-traversal): " + wd)
	}

	timeout := defaultTimeout
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}
	if spec.TimeoutSec > 0 {
		timeout = time.Duration(spec.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec — argv (no shell), command dari adapter.json owner-installed
	cmd.Dir = full
	cmd.Env = buildEnv(cfg.Env)
	if stdinData != nil {
		cmd.Stdin = bytes.NewReader(stdinData)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exit := 0
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("op timeout (%s)", timeout)
	}
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exit = ee.ExitCode()
		} else {
			// program ga ketemu / ga bisa start = error nyata (bukan exit-code app).
			return nil, fmt.Errorf("jalanin %q: %w", argv[0], runErr)
		}
	}
	return map[string]any{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
		"exit":   exit,
	}, nil
}

// substitute — ganti {key} di s dengan args[key] (string). Tandai key kepakai.
func substitute(s string, args map[string]any, used map[string]bool) string {
	for k, v := range args {
		ph := "{" + k + "}"
		if strings.Contains(s, ph) {
			s = strings.ReplaceAll(s, ph, toStr(v))
			used[k] = true
		}
	}
	return s
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case float64:
		// JSON number: cetak tanpa .0 yang ga perlu.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "."
}

// buildEnv — env host + env tambahan dari adapter.json (white-label brand dari sini).
func buildEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func writeResult(w io.Writer, result any, stateVer int64) {
	raw, err := json.Marshal(result)
	if err != nil {
		writeError(w, "marshal result: "+err.Error())
		return
	}
	resp, _ := json.Marshal(map[string]any{"result": json.RawMessage(raw), "state_version": stateVer})
	_, _ = w.Write(append(resp, '\n'))
}

func writeError(w io.Writer, msg string) {
	resp, _ := json.Marshal(map[string]any{"error": msg})
	_, _ = w.Write(append(resp, '\n'))
}
