package loket

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGUIEmitLatest(t *testing.T) {
	g := newGUIProviders()
	if _, err := g.emit(context.Background(), "m", json.RawMessage(`{"panel":"p","data":{"x":1}}`)); err != nil {
		t.Fatalf("emit: %v", err)
	}
	e, ok := g.Latest("m", "p")
	if !ok || string(e.Data) != `{"x":1}` {
		t.Errorf("latest wrong: ok=%v data=%s", ok, e.Data)
	}
	// isolation: another module/panel is empty
	if _, ok := g.Latest("other", "p"); ok {
		t.Error("other module should have no data")
	}
}
