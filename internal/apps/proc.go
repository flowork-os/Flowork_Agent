// proc.go — adapter PROSES untuk app core LINTAS BAHASA (ROADMAP 4 §5.4). Sebuah app core
// runtime:process = perintah (python/node/ruby/biner C/…) yang ngomong protokol baris-JSON di
// stdio: kita kirim {"op":...,"args":...}\n, dia balas {"result":...,"state_version":N}\n.
// Bahasa core BEBAS — yang penting bisa baca/tulis JSON di stdin/stdout. Proses TETAP HIDUP
// (state ada di memorinya). Serial per-app (1 request pada satu waktu).
package apps

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type proc struct {
	mu    sync.Mutex
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *bufio.Reader
}

// startProc — spawn core di folder app. command mis. "python3 core.py".
func startProc(command, dir string) (*proc, error) {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return nil, fmt.Errorf("core_entry kosong")
	}
	cmd := exec.Command(parts[0], parts[1:]...) // #nosec — command dari manifest app owner-installed (trusted, §15)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &proc{cmd: cmd, stdin: stdin, out: bufio.NewReaderSize(stdout, 1<<20)}, nil
}

// call — kirim 1 operasi, tunggu 1 balasan (timeout). Serial.
func (p *proc) call(op string, args json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	req, _ := json.Marshal(map[string]any{"op": op, "args": args})
	if _, err := p.stdin.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("write core: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type res struct {
		line []byte
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		line, err := p.out.ReadBytes('\n')
		ch <- res{line, err}
	}()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("core timeout op=%s", op)
	case r := <-ch:
		if r.err != nil && len(r.line) == 0 {
			return nil, fmt.Errorf("read core: %w", r.err)
		}
		return json.RawMessage(r.line), nil
	}
}

func (p *proc) close() {
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}
