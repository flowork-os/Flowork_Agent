// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	server *Server
	cm     *CertManager
	addr   string
	hosts  []string
	mu     sync.Mutex
}

func NewManager(addr string, cm *CertManager, hosts []string) *Manager {
	return &Manager{
		addr:  addr,
		cm:    cm,
		hosts: hosts,
	}
}

func PidFile() string { return filepath.Join(MITMDir(), ".mitm.pid") }

func (m *Manager) Start(handler interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return errors.New("manager already started")
	}
	if err := os.MkdirAll(MITMDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir mitm: %w", err)
	}
	if err := os.WriteFile(PidFile(), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write pidfile: %w", err)
	}
	if len(m.hosts) > 0 {
		if err := AddDNSEntries(m.hosts); err != nil {

			_ = os.Remove(PidFile())
			return fmt.Errorf("dns hijack: %w", err)
		}
	}
	srv := NewServer(m.addr, m.cm, nil)
	m.server = srv
	go func() { _ = srv.Start() }()
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.server.Shutdown(ctx)
	m.server = nil
	_ = RemoveAllDNSEntries()
	_ = os.Remove(PidFile())
	return nil
}

func ReadPidFile() int {
	b, err := os.ReadFile(PidFile())
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return p
}

func IsRunning() bool {
	pid := ReadPidFile()
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	return p.Signal(syscall_zero()) == nil
}
