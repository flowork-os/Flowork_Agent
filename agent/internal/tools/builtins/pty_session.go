// pty_session.go — SIBLING ext (⚠️ FROZEN 2026-07-02 seizin owner — stabil+live): exec INTERAKTIF lewat PTY
// (roadmap "buka lock": PTY buat exec interaktif). Agent bisa jalanin program yang
// butuh terminal (REPL python/sqlite, prompt interaktif) — start sesi, kirim input,
// baca output, tutup. Plug-and-play: init() self-register (NOL sentuh builtins.go).
//
// KEAMANAN (§8.2): powerful primitive → default OFF (switch FLOWORK_PTY=1 buat nyalain).
// Tiap perintah start + tiap input di-guard `classifyCommand` (shell_guard). Kerja
// DIKURUNG di workspace agent. Sesi auto-reap kalau idle. Bekuin pas udah teruji.
// 📄 Dok: lock/pty-exec.md
package builtins

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/tools"
)

// ptyEnabled — switch FLOWORK_PTY. Default OFF (opt-in). OFF → tool ga ke-register.
func ptyEnabled() bool {
	v := strings.TrimSpace(os.Getenv("FLOWORK_PTY"))
	return v == "1" || strings.EqualFold(v, "true")
}

func init() {
	if !ptyEnabled() {
		return
	}
	tools.Register(&ptyStartTool{})
	tools.Register(&ptySendTool{})
	tools.Register(&ptyReadTool{})
	tools.Register(&ptyCloseTool{})
}

const (
	ptyMaxSessions = 8                // batasi sesi hidup barengan (anti-leak)
	ptyIdleTimeout = 10 * time.Minute // sesi nganggur > ini → di-reap
	ptyBufCap      = 256 * 1024       // ring output per sesi (256KB, buang paling lama)
)

type ptySession struct {
	id      string
	cmd     *exec.Cmd
	master  *os.File
	mu      sync.Mutex
	buf     bytes.Buffer
	done    bool // proses udah exit (reader EOF)
	lastUse time.Time
}

var (
	ptyMu      sync.Mutex
	ptySeq     int
	ptyStore   = map[string]*ptySession{}
	ptyReaper  sync.Once
)

// appendOutput — reader goroutine nempelin output ke buffer (di-cap ring).
func (s *ptySession) appendOutput(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf.Len()+len(p) > ptyBufCap {
		// buang paling lama: sisain ekor.
		keep := ptyBufCap - len(p)
		if keep < 0 {
			keep = 0
		}
		b := s.buf.Bytes()
		if len(b) > keep {
			tail := append([]byte{}, b[len(b)-keep:]...)
			s.buf.Reset()
			s.buf.Write(tail)
		}
	}
	s.buf.Write(p)
}

// drain — ambil + kosongin output yang numpuk.
func (s *ptySession) drain() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.buf.String()
	s.buf.Reset()
	return out
}

func ptyGet(id string) (*ptySession, bool) {
	ptyMu.Lock()
	defer ptyMu.Unlock()
	s, ok := ptyStore[strings.TrimSpace(id)]
	return s, ok
}

func ptyRemove(id string) {
	ptyMu.Lock()
	defer ptyMu.Unlock()
	delete(ptyStore, id)
}

// startReaper — goroutine tunggal yang nutup sesi idle. Dijalanin sekali (lazy).
func startReaper() {
	ptyReaper.Do(func() {
		go func() {
			t := time.NewTicker(time.Minute)
			defer t.Stop()
			for range t.C {
				now := time.Now()
				var stale []*ptySession
				ptyMu.Lock()
				for _, s := range ptyStore {
					s.mu.Lock()
					idle := now.Sub(s.lastUse) > ptyIdleTimeout
					s.mu.Unlock()
					if idle {
						stale = append(stale, s)
					}
				}
				ptyMu.Unlock()
				for _, s := range stale {
					s.kill()
					ptyRemove(s.id)
				}
			}
		}()
	})
}

func (s *ptySession) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.master != nil {
		_ = s.master.Close()
	}
}

// =============================================================================
// pty_start
// =============================================================================

type ptyStartTool struct{}

func (ptyStartTool) Name() string       { return "pty_start" }
func (ptyStartTool) Capability() string { return "exec:shell" }
func (ptyStartTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Start an INTERACTIVE program under a pseudo-terminal (PTY) — for REPLs / TUIs " +
			"that need a real terminal (python3, sqlite3, an interactive shell). Returns session_id + " +
			"first output. Use pty_send to type, pty_read to read more, pty_close to end. Guarded like shell.",
		Params: []tools.Param{
			{Name: "command", Type: tools.ParamString, Description: "program to run (e.g. 'python3', 'sqlite3 db.sqlite'). Empty = interactive shell."},
			{Name: "wait_ms", Type: tools.ParamInt, Description: "how long to wait for first output before returning (default 400, max 5000)"},
		},
		Returns: "{session_id, output, done}",
	}
}

func (ptyStartTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	command := strings.TrimSpace(nbArgString(args, "command"))
	if command != "" {
		if blocked, reason, _ := classifyCommand(command); blocked {
			return tools.Result{}, fmt.Errorf("pty_start: blocked — %s", reason)
		}
	}
	ptyMu.Lock()
	if len(ptyStore) >= ptyMaxSessions {
		ptyMu.Unlock()
		return tools.Result{}, fmt.Errorf("pty_start: kebanyakan sesi hidup (%d) — tutup dulu pakai pty_close", ptyMaxSessions)
	}
	ptySeq++
	id := "pty-" + strconv.Itoa(ptySeq)
	ptyMu.Unlock()

	workdir := tools.FromSharedDir(ctx)
	s, err := startPTYSession(id, command, workdir)
	if err != nil {
		return tools.Result{}, fmt.Errorf("pty_start: %w", err)
	}
	ptyMu.Lock()
	ptyStore[id] = s
	ptyMu.Unlock()
	startReaper()

	wait := argIntDefault(args, "wait_ms", 400, 5000)
	time.Sleep(time.Duration(wait) * time.Millisecond)
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	return tools.Result{
		Output: map[string]any{"session_id": id, "output": s.drain(), "done": done},
		Note:   "sesi PTY " + id + " jalan — pty_send buat ngetik, pty_close buat nutup",
	}, nil
}

// =============================================================================
// pty_send
// =============================================================================

type ptySendTool struct{}

func (ptySendTool) Name() string       { return "pty_send" }
func (ptySendTool) Capability() string { return "exec:shell" }
func (ptySendTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Send input (a line of text) to a running PTY session's stdin, then read the reply. " +
			"A newline is appended unless newline=false. Input is guarded like a shell command.",
		Params: []tools.Param{
			{Name: "session_id", Type: tools.ParamString, Description: "id from pty_start", Required: true},
			{Name: "input", Type: tools.ParamString, Description: "text to type", Required: true},
			{Name: "newline", Type: tools.ParamBool, Description: "append Enter (default true)"},
			{Name: "wait_ms", Type: tools.ParamInt, Description: "wait for output after sending (default 400, max 5000)"},
		},
		Returns: "{output, done}",
	}
}

func (ptySendTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	id := strings.TrimSpace(nbArgString(args, "session_id"))
	s, ok := ptyGet(id)
	if !ok {
		return tools.Result{}, fmt.Errorf("pty_send: sesi %q ga ketemu (udah ditutup?)", id)
	}
	input := nbArgString(args, "input")
	if blocked, reason, _ := classifyCommand(input); blocked {
		return tools.Result{}, fmt.Errorf("pty_send: blocked — %s", reason)
	}
	newline := true
	if v, ok := args["newline"].(bool); ok {
		newline = v
	}
	payload := input
	if newline {
		payload += "\n"
	}
	s.mu.Lock()
	s.lastUse = time.Now()
	done := s.done
	s.mu.Unlock()
	if done {
		return tools.Result{}, fmt.Errorf("pty_send: proses sesi %q udah exit", id)
	}
	if _, err := s.master.Write([]byte(payload)); err != nil {
		return tools.Result{}, fmt.Errorf("pty_send: tulis stdin: %w", err)
	}
	wait := argIntDefault(args, "wait_ms", 400, 5000)
	time.Sleep(time.Duration(wait) * time.Millisecond)
	s.mu.Lock()
	done = s.done
	s.mu.Unlock()
	return tools.Result{Output: map[string]any{"output": s.drain(), "done": done}}, nil
}

// =============================================================================
// pty_read
// =============================================================================

type ptyReadTool struct{}

func (ptyReadTool) Name() string       { return "pty_read" }
func (ptyReadTool) Capability() string { return "exec:shell" }
func (ptyReadTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Read any output accumulated from a PTY session since the last read (non-blocking, " +
			"optionally waits briefly). Use after pty_start/pty_send when a program keeps printing.",
		Params: []tools.Param{
			{Name: "session_id", Type: tools.ParamString, Description: "id from pty_start", Required: true},
			{Name: "wait_ms", Type: tools.ParamInt, Description: "wait for more output first (default 0, max 5000)"},
		},
		Returns: "{output, done}",
	}
}

func (ptyReadTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	id := strings.TrimSpace(nbArgString(args, "session_id"))
	s, ok := ptyGet(id)
	if !ok {
		return tools.Result{}, fmt.Errorf("pty_read: sesi %q ga ketemu", id)
	}
	if wait := argIntDefault(args, "wait_ms", 0, 5000); wait > 0 {
		time.Sleep(time.Duration(wait) * time.Millisecond)
	}
	s.mu.Lock()
	s.lastUse = time.Now()
	done := s.done
	s.mu.Unlock()
	return tools.Result{Output: map[string]any{"output": s.drain(), "done": done}}, nil
}

// =============================================================================
// pty_close
// =============================================================================

type ptyCloseTool struct{}

func (ptyCloseTool) Name() string       { return "pty_close" }
func (ptyCloseTool) Capability() string { return "exec:shell" }
func (ptyCloseTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Terminate a PTY session and free it. Always close sessions you no longer need.",
		Params: []tools.Param{
			{Name: "session_id", Type: tools.ParamString, Description: "id from pty_start", Required: true},
		},
		Returns: "{closed, final_output}",
	}
}

func (ptyCloseTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	id := strings.TrimSpace(nbArgString(args, "session_id"))
	s, ok := ptyGet(id)
	if !ok {
		return tools.Result{}, fmt.Errorf("pty_close: sesi %q ga ketemu", id)
	}
	final := s.drain()
	s.kill()
	ptyRemove(id)
	return tools.Result{Output: map[string]any{"closed": true, "final_output": final}}, nil
}

// argIntDefault — ambil arg int (via argInt), default def, clamp ke [0,max].
func argIntDefault(args map[string]any, key string, def, max int) int {
	n := def
	if v, ok := argInt(args, key); ok {
		n = int(v)
	}
	if n < 0 {
		n = 0
	}
	if n > max {
		n = max
	}
	return n
}
