package loket

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// guiProviders back gui.emit (§2.F): a module pushes live data to its OWN declared
// panel. The kernel keeps only the LATEST snapshot per (module, panel) in memory;
// the GUI reads it back through an endpoint and renders it with the trusted widget
// vocabulary (§8.I — frontend rendering is a separate piece). The store key is the
// kernel-VERIFIED caller id, so a module can only write its own panels — never
// another module's. This is the backend half of the declarative-GUI contract.
type guiProviders struct {
	mu   sync.RWMutex
	data map[string]guiEntry // key = module + "\x00" + panel
}

type guiEntry struct {
	Module string          `json:"module"`
	Panel  string          `json:"panel"`
	Data   json.RawMessage `json:"data"`
	TS     string          `json:"ts"`
}

func newGUIProviders() *guiProviders {
	return &guiProviders{data: map[string]guiEntry{}}
}

func guiKey(module, panel string) string { return module + "\x00" + panel }

func (g *guiProviders) emit(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Panel string          `json:"panel"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.Panel == "" {
		a.Panel = "default"
	}
	if len(a.Data) == 0 {
		a.Data = json.RawMessage("{}")
	}
	g.mu.Lock()
	g.data[guiKey(module, a.Panel)] = guiEntry{
		Module: module, Panel: a.Panel, Data: a.Data,
		TS: time.Now().UTC().Format(time.RFC3339),
	}
	g.mu.Unlock()
	return mustJSON(map[string]any{"ok": true}), nil
}

// Latest returns the most recent snapshot a module pushed to a panel.
func (g *guiProviders) Latest(module, panel string) (guiEntry, bool) {
	if panel == "" {
		panel = "default"
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	e, ok := g.data[guiKey(module, panel)]
	return e, ok
}
