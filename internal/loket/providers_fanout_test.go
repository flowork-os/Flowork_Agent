package loket

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// TestBroadcastFanout covers the plug-and-play coordination hook: the serial default
// (hook nil) and a registered strategy (used + collects all), incl. a parallel one.
func TestBroadcastFanout(t *testing.T) {
	mockInvoke := func(_ context.Context, target string, _ Message) (json.RawMessage, error) {
		return json.RawMessage(`"` + target + `"`), nil
	}
	bp := &busProviders{deps: Deps{Invoke: mockInvoke}}
	args := json.RawMessage(`{"to":["a","b","c"],"type":"task","payload":{}}`)
	parse := func(raw json.RawMessage) int {
		var g struct {
			Replies []FanoutBroadcastReply `json:"replies"`
		}
		_ = json.Unmarshal(raw, &g)
		return len(g.Replies)
	}

	old := FanoutStrategy
	defer func() { FanoutStrategy = old }()

	// serial default
	FanoutStrategy = nil
	r, err := bp.broadcast(context.Background(), "caller", args)
	if err != nil {
		t.Fatal(err)
	}
	if n := parse(r); n != 3 {
		t.Fatalf("serial: want 3 replies, got %d", n)
	}

	// pluggable PARALLEL strategy: used + collects all targets
	used := false
	FanoutStrategy = func(targets []string, invoke func(string) (json.RawMessage, error)) []FanoutBroadcastReply {
		used = true
		out := make([]FanoutBroadcastReply, len(targets))
		var wg sync.WaitGroup
		for i, tg := range targets {
			wg.Add(1)
			go func(idx int, target string) {
				defer wg.Done()
				rr, _ := invoke(target)
				out[idx] = FanoutBroadcastReply{Target: target, Reply: rr}
			}(i, tg)
		}
		wg.Wait()
		return out
	}
	r2, err := bp.broadcast(context.Background(), "caller", args)
	if err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Error("FanoutStrategy hook was not used")
	}
	if n := parse(r2); n != 3 {
		t.Fatalf("parallel: want 3 replies, got %d", n)
	}
}
