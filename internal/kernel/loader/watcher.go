// Watcher — fsnotify hook supaya kernel auto-detect plugin baru atau yang
// dihapus tanpa restart manual. Phase 12 add-on; kernel main.go spawn satu
// instance ini di goroutine kalau caller pass NewWatcher.

package loader

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ChangeKind — kind perubahan yang watcher emit ke listener.
type ChangeKind string

const (
	ChangeAdded   ChangeKind = "added"
	ChangeRemoved ChangeKind = "removed"
	ChangeUpdated ChangeKind = "updated"
)

// Change — payload event yang dikirim ke listener channel.
type Change struct {
	Kind    ChangeKind
	Path    string // absolute path ke <id>.fwagent folder
	AgentID string // dari nama folder (path basename minus ".fwagent")
}

// Watcher subscribe ke fsnotify events di pluginsDir + debounce 500 ms
// supaya satu unzip yang muncul ratusan event tidak men-trigger banyak
// re-load. listener nerima satu Change per debounce window.
type Watcher struct {
	dir      string
	listener chan Change
	w        *fsnotify.Watcher

	debounceMu sync.Mutex
	debounceAt map[string]time.Time
}

// NewWatcher buka fsnotify dan return Watcher. Kalau platform tidak
// support fsnotify (rare di Linux/macOS/Windows pure-Go), error.
func NewWatcher(dir string) (*Watcher, error) {
	if dir == "" {
		return nil, errors.New("dir required")
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}
	return &Watcher{
		dir:        dir,
		listener:   make(chan Change, 8),
		w:          w,
		debounceAt: map[string]time.Time{},
	}, nil
}

// Listener returns the channel kernel main.go select-on untuk dapet
// changes. Channel close kalau watcher exit.
func (w *Watcher) Listener() <-chan Change {
	return w.listener
}

// Run loop sampai ctx cancel. Non-blocking — caller pakai di goroutine.
func (w *Watcher) Run(ctx context.Context) {
	defer w.w.Close()
	defer close(w.listener)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			log.Printf("plugin watcher error: %v", err)
		}
	}
}

// handle convert fsnotify event ke ChangeKind. Debounce per-folder
// supaya unzip / copy yang spawn multiple writes cuma trigger sekali.
func (w *Watcher) handle(ev fsnotify.Event) {
	base := filepath.Base(ev.Name)
	if !strings.HasSuffix(base, ".fwagent") {
		// Mungkin file di dalam folder agent — naik satu level.
		parent := filepath.Base(filepath.Dir(ev.Name))
		if !strings.HasSuffix(parent, ".fwagent") {
			return
		}
		base = parent
	}
	path := filepath.Join(w.dir, base)
	agentID := strings.TrimSuffix(base, ".fwagent")

	kind := ChangeUpdated
	switch {
	case ev.Op&fsnotify.Create != 0:
		kind = ChangeAdded
	case ev.Op&fsnotify.Remove != 0, ev.Op&fsnotify.Rename != 0:
		kind = ChangeRemoved
	case ev.Op&fsnotify.Write != 0:
		kind = ChangeUpdated
	}

	// Debounce 1500 ms per (path, kind). Unzip/copy yang spawn banyak event
	// flapping selama beberapa ratus ms; tunggu sampai sumber tenang biar
	// kernel lihat state final (semua file selesai ditulis).
	key := string(kind) + "|" + path
	w.debounceMu.Lock()
	last, ok := w.debounceAt[key]
	now := time.Now()
	if ok && now.Sub(last) < 1500*time.Millisecond {
		w.debounceMu.Unlock()
		return
	}
	w.debounceAt[key] = now
	w.debounceMu.Unlock()

	select {
	case w.listener <- Change{Kind: kind, Path: path, AgentID: agentID}:
	default:
		// listener buffer full — skip biar gak block; next event masih
		// kena debounce, jadi state akhirnya tetap consistent.
	}
}
