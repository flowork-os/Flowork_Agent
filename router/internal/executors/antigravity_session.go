// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"crypto/rand"
	"strconv"
	"sync"
	"time"
)

const (
	sessionTTL = 2 * time.Hour

	sessionCleanupInterval = 30 * time.Minute

	sessionMaxEntries = 1000
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

func startSessionSweeper() {
	sessionInit.Do(func() {
		go func() {
			t := time.NewTicker(sessionCleanupInterval)
			defer t.Stop()
			for range t.C {
				sessionMu.Lock()
				now := time.Now()
				for k, e := range sessionStore {
					if now.Sub(e.lastUsed) > sessionTTL {
						delete(sessionStore, k)
					}
				}
				sessionMu.Unlock()
			}
		}()
	})
}

func DeriveAntigravitySessionID(connectionID string) string {
	startSessionSweeper()
	if connectionID == "" {
		return GenerateAntigravitySessionID()
	}
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if e, ok := sessionStore[connectionID]; ok {
		e.lastUsed = time.Now()
		return e.id
	}

	if len(sessionStore) >= sessionMaxEntries {
		var oldestKey string
		var oldestTime time.Time
		for k, e := range sessionStore {
			if oldestKey == "" || e.lastUsed.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.lastUsed
			}
		}
		delete(sessionStore, oldestKey)
	}
	id := GenerateAntigravitySessionID()
	sessionStore[connectionID] = &sessionEntry{id: id, lastUsed: time.Now()}
	return id
}

func GenerateAntigravitySessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0F) | 0x40
	b[8] = (b[8] & 0x3F) | 0x80
	const hexd = "0123456789abcdef"
	out := make([]byte, 36)
	bi := 0
	for i := 0; i < 36; i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			out[i] = '-'
			continue
		}
		out[i] = hexd[b[bi]>>4]
		i++
		out[i] = hexd[b[bi]&0xF]
		bi++
	}
	return string(out) + strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func ClearAntigravitySessionStore() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionStore = map[string]*sessionEntry{}
}

func AntigravitySessionStoreSize() int {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	return len(sessionStore)
}
