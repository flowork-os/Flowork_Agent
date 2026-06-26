// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package streamutil

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	sessionTTL             = 12 * time.Hour
	sessionCleanupInterval = 30 * time.Minute
)

type sessionEntry struct {
	id       string
	lastUsed time.Time
}

var (
	sessionMu    sync.Mutex
	sessionStore = map[string]*sessionEntry{}
	sessionInit  sync.Once
)

func startSessionCleanup() {
	go func() {
		ticker := time.NewTicker(sessionCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-sessionTTL)
			sessionMu.Lock()
			for k, e := range sessionStore {
				if e.lastUsed.Before(cutoff) {
					delete(sessionStore, k)
				}
			}
			sessionMu.Unlock()
		}
	}()
}

func DeriveSessionID(connectionID string) string {
	if connectionID == "" {
		return ""
	}
	sessionInit.Do(startSessionCleanup)
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if e := sessionStore[connectionID]; e != nil {
		e.lastUsed = time.Now()
		return e.id
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	sessionStore[connectionID] = &sessionEntry{id: id, lastUsed: time.Now()}
	return id
}

func ResetSessionID(connectionID string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	delete(sessionStore, connectionID)
}

func ActiveSessionCount() int {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	return len(sessionStore)
}
