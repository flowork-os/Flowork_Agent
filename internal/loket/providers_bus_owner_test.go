package loket

import (
	"context"
	"encoding/json"
	"testing"
)

func TestBusSendOwner(t *testing.T) {
	got := ""
	bp := &busProviders{deps: Deps{NotifyOwner: func(_ context.Context, text string) error {
		got = text
		return nil
	}}}
	if _, err := bp.send(context.Background(), "m", json.RawMessage(`{"to":"owner","payload":{"text":"halo owner"}}`)); err != nil {
		t.Fatalf("send owner: %v", err)
	}
	if got != "halo owner" {
		t.Errorf("owner text wrong: %q", got)
	}
	// owner send with no NotifyOwner wired → error
	bp2 := &busProviders{deps: Deps{}}
	if _, err := bp2.send(context.Background(), "m", json.RawMessage(`{"to":"owner","payload":{"text":"x"}}`)); err == nil {
		t.Error("owner send without NotifyOwner should error")
	}
}
