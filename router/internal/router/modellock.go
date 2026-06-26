// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/services"
	"github.com/flowork-os/flowork_Router/internal/store"
)

type modelLockEntry struct {
	until        time.Time
	backoffLevel int
}

var (
	modelLocks = map[string]modelLockEntry{}
	mlMu       sync.Mutex
)

func modelLockKey(providerID, model string) string {
	return providerID + "\x00" + model
}

func isModelLocked(providerID, model string) bool {
	mlMu.Lock()
	defer mlMu.Unlock()
	e, ok := modelLocks[modelLockKey(providerID, model)]
	if !ok {
		return false
	}
	if time.Now().After(e.until) {
		delete(modelLocks, modelLockKey(providerID, model))
		return false
	}
	return true
}

func lockModel(providerID, model string, status int, errText string) {
	mlMu.Lock()
	defer mlMu.Unlock()
	key := modelLockKey(providerID, model)
	prev := modelLocks[key].backoffLevel
	dec := services.CheckFallbackError(status, errText, prev)
	if !dec.ShouldFallback || dec.Cooldown <= 0 {
		return
	}
	modelLocks[key] = modelLockEntry{
		until:        time.Now().Add(dec.Cooldown),
		backoffLevel: dec.NewBackoffLevel,
	}
}

func clearModelLock(providerID, model string) {
	mlMu.Lock()
	defer mlMu.Unlock()
	delete(modelLocks, modelLockKey(providerID, model))
}

func reorderByModelLock(matches []store.ProviderConnection, model string) []store.ProviderConnection {
	if len(matches) < 2 {
		return matches
	}
	avail := make([]store.ProviderConnection, 0, len(matches))
	locked := make([]store.ProviderConnection, 0)
	for _, p := range matches {
		if isModelLocked(p.ID, model) {
			locked = append(locked, p)
		} else {
			avail = append(avail, p)
		}
	}
	if len(locked) == 0 {
		return matches
	}
	return append(avail, locked...)
}
