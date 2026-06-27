// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: WASM instance per-call wrapper. Audit pass — atomic counter unique
//   module name (anti race), per-call WASI re-instantiate, FS preopen mount
//   /workspace + /shared (sandbox), stderr cap 200 chars, output copy (no
//   alias), compile error %w, empty input guards, ExitError(0) success path,
//   stderr tee realtime.
//
// Instance wrapper — plugin compiled module yang siap di-instantiate
// per call (command pattern). Pattern ini bypass keterbatasan TinyGo
// wasmexport yang panic setelah _start exit; setiap call jadi lifecycle
// penuh (instantiate → _start → main → exit → close), dengan WASI args
// pass function + JSON arg, WASI stdout capture response.

package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/tetratelabs/wazero"
)

// callCounter — global atomic untuk generate unique module name per call.
// Wazero refuse instantiate dengan nama duplicate; long-poll daemon
// + concurrent send_message RPC butuh nama unik supaya kedua module
// hidup bareng tanpa tabrakan.
var callCounter uint64

// Instance — handle ke agent yang sudah ter-compile.
// CompiledModule disimpan supaya instantiate cuma ~1ms per call (skip
// parse + validate WASM bytes).
type Instance struct {
	ID       string
	rt       *Runtime
	compiled wazero.CompiledModule
	memMaxMB int
	// env yang kernel inject ke setiap instantiate (mis. bot token,
	// allowed chat IDs). Agent baca via os.Getenv.
	env map[string]string
	// workspaceDir = ~/.flowork/agents/<id>.fwagent/workspace/, mounted
	// di /workspace; per-agent terisolasi total.
	workspaceDir string
	// sharedDir = ~/.flowork/shared/, mounted di /shared kalau agent
	// declare capability "fs:shared". Kosong = ngga di-mount.
	sharedDir string
}

// SetEnv set env vars yang akan di-pass ke setiap instantiate agent.
func (i *Instance) SetEnv(env map[string]string) {
	i.env = make(map[string]string, len(env))
	for k, v := range env {
		i.env[k] = v
	}
}

// SetWorkspaces mount per-agent + shared workspace directories.
// Kosongin sharedDir kalau agent ngga punya cap fs:shared.
func (i *Instance) SetWorkspaces(workspaceDir, sharedDir string) {
	i.workspaceDir = workspaceDir
	i.sharedDir = sharedDir
}

// Call instantiate ulang module dengan args = [plugin, function, args-json],
// jalanin _start (yang call main), tangkap stdout sebagai response.
// Setelah module close, semua state plugin diset bersih lagi.
//
//   ctx        — caller context (akan di-augment dengan pluginID)
//   funcName   — nama function dari manifest.exposes_rpc[].name
//   argsJSON   — request JSON bytes ready-to-send
//
// Return: JSON response bytes (sudah trim trailing newline dari println).
func (i *Instance) Call(ctx context.Context, funcName string, argsJSON []byte) (result []byte, retErr error) {
	if i.compiled == nil {
		return nil, fmt.Errorf("instance %q closed", i.ID)
	}
	// PANIC-ISOLATION (akar server-crash): WASM engine (wazevo) bisa panic di kondisi
	// pinggir — mis. race poll_oneoff pas daemon long-poll ngebut waktu network down
	// (mod.Sys nil di pollOneoffFn). 1 panic plugin TIDAK BOLEH matiin SELURUH agent.
	// recover → ubah jadi error biasa (caller udah handle err), server tetap hidup.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("plugin call %q panicked (WASM engine): %v", funcName, r)
			fmt.Fprintf(os.Stderr, "[kernel] RECOVERED panic: plugin %q func %q: %v\n", i.ID, funcName, r)
		}
	}()
	callCtx := WithGuestPluginID(ctx, i.ID)

	var stdout, stderr bytes.Buffer
	// Wazero rejects duplicate module names. Long-poll daemon + concurrent
	// send_message RPC butuh dua instance plugin hidup bareng, jadi tiap
	// instantiate dapet nama unik.
	seq := atomic.AddUint64(&callCounter, 1)
	// Stderr tee ke kernel stderr supaya log plugin daemon kelihatan
	// realtime di flowork-agent.log (bytes.Buffer cuma flush saat main
	// exit; daemon long-poll never exits → stderr trapped).
	stderrTee := io.MultiWriter(&stderr, os.Stderr)
	cfg := wazero.NewModuleConfig().
		WithName(fmt.Sprintf("%s#%d", i.ID, seq)).
		WithStartFunctions("_start").
		WithArgs("agent", funcName, string(argsJSON)).
		WithStdout(&stdout).
		WithStderr(stderrTee)
	for k, v := range i.env {
		cfg = cfg.WithEnv(k, v)
	}
	// Mount workspaces via WASI preopen. Agent buka `/workspace/foo.json`
	// → real path `~/.flowork/agents/<id>.fwagent/workspace/foo.json`.
	// Isolation: tiap agent cuma lihat dirinya sendiri.
	if i.workspaceDir != "" || i.sharedDir != "" {
		fs := wazero.NewFSConfig()
		if i.workspaceDir != "" {
			fs = fs.WithDirMount(i.workspaceDir, "/workspace")
		}
		if i.sharedDir != "" {
			fs = fs.WithDirMount(i.sharedDir, "/shared")
		}
		cfg = cfg.WithFSConfig(fs)
	}

	mod, err := i.rt.rt.InstantiateModule(callCtx, i.compiled, cfg)
	if err != nil {
		// Module bisa exit non-zero via os.Exit(N); wazero return
		// ExitError. Treat ExitError exit-code 0 sebagai sukses jika
		// stdout-nya valid JSON; selain itu surface error apa adanya.
		if !isCleanExit(err) {
			return nil, fmt.Errorf("plugin call %q: %w (stderr=%q)", funcName, err, trim(stderr.String()))
		}
	} else {
		_ = mod.Close(callCtx)
	}

	out := bytes.TrimRight(stdout.Bytes(), "\n")
	if len(out) == 0 {
		// Kalau stderr berisi pesan, propagate sebagai error context.
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("plugin %q produced no stdout (stderr=%q)", i.ID, trim(stderr.String()))
		}
		return nil, nil
	}
	cp := make([]byte, len(out))
	copy(cp, out)
	return cp, nil
}

// isCleanExit — wazero return error untuk sys.ExitError dengan code 0
// pada saat _start return ke proc_exit. Treat as success.
func isCleanExit(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "exit_code(0)")
}

func trim(s string) string {
	const max = 200
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// LoadInstance compile module dan simpan ke registry. Tidak instantiate
// langsung; instantiate happens per Call. Return Instance handle yang
// caller pegang untuk dispatch.
func (r *Runtime) LoadInstance(ctx context.Context, pluginID string, wasm []byte, memMaxMiB int) (*Instance, error) {
	if pluginID == "" {
		return nil, fmt.Errorf("plugin id required")
	}
	if len(wasm) == 0 {
		return nil, fmt.Errorf("empty wasm")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.instances[pluginID]; exists {
		return nil, ErrAlreadyLoaded
	}
	compiled, err := r.rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", pluginID, err)
	}
	inst := &Instance{
		ID:       pluginID,
		rt:       r,
		compiled: compiled,
		memMaxMB: memMaxMiB,
	}
	r.instances[pluginID] = inst
	return inst, nil
}

// Get returns the Instance untuk plugin ID, atau nil kalau belum diload.
func (r *Runtime) Get(pluginID string) *Instance {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.instances[pluginID]
}
