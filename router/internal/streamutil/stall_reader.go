// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package streamutil

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultStallTimeout = 35 * time.Second

var ErrStreamStall = errors.New("stream stalled: no data within timeout")

type StallReader struct {
	src     io.ReadCloser
	timeout time.Duration
	mu      sync.Mutex
	stalled atomic.Bool
	closed  atomic.Bool
	cancel  chan struct{}
}

func NewStallReader(src io.ReadCloser, timeout time.Duration) *StallReader {
	return &StallReader{
		src:     src,
		timeout: timeout,
		cancel:  make(chan struct{}),
	}
}

func (r *StallReader) Read(p []byte) (int, error) {
	if r.stalled.Load() {
		return 0, ErrStreamStall
	}
	if r.timeout <= 0 {
		return r.src.Read(p)
	}

	timer := time.AfterFunc(r.timeout, func() {
		if !r.stalled.CompareAndSwap(false, true) {
			return
		}
		_ = r.src.Close()
	})

	n, err := r.src.Read(p)
	timer.Stop()

	if r.stalled.Load() {
		return n, ErrStreamStall
	}
	return n, err
}

func (r *StallReader) Close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(r.cancel)
	return r.src.Close()
}

func (r *StallReader) HasStalled() bool { return r.stalled.Load() }
