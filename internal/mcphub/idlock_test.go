package mcphub

import (
	"runtime"
	"sync"
	"testing"

	"flowork-gui/internal/mcpclient"
)

// TestIDLockSerializesSameID proves the per-connector lock that guards
// Enable/Disable/Uninstall: the same id returns the same lock, different ids get
// different locks, and the lock actually serializes concurrent critical sections for
// one id (run with -race). This is what stops two concurrent Enable(id) calls from
// double-spawning a server and orphaning a process.
func TestIDLockSerializesSameID(t *testing.T) {
	m := &Manager{
		servers: map[string]*mcpclient.Server{},
		regs:    map[string][]string{},
		locks:   map[string]*sync.Mutex{},
	}
	if m.idLock("github") != m.idLock("github") {
		t.Fatal("same id must return the same lock instance")
	}
	if m.idLock("github") == m.idLock("gitlab") {
		t.Fatal("different ids must get different lock instances")
	}

	var counter int
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lk := m.idLock("x")
			lk.Lock()
			c := counter
			runtime.Gosched() // widen the window a racy update would lose
			counter = c + 1
			lk.Unlock()
		}()
	}
	wg.Wait()
	if counter != 50 {
		t.Fatalf("per-id lock failed to serialize: counter=%d want 50", counter)
	}
}
