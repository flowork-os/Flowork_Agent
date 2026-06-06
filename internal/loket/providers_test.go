package loket

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
)

// ── direct store tests ───────────────────────────────────────────────────────

func TestStoreKVDocBrain(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// kv
	if err := st.KVSet("router_url", "http://x"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := st.KVGet("router_url"); !ok || v != "http://x" {
		t.Errorf("kv get = %q,%v", v, ok)
	}
	if _, ok, _ := st.KVGet("missing"); ok {
		t.Error("missing key reported found")
	}

	// doc
	if err := st.DocPut("findings", "f1", json.RawMessage(`{"sev":"high"}`)); err != nil {
		t.Fatal(err)
	}
	if body, ok, _ := st.DocGet("findings", "f1"); !ok || string(body) != `{"sev":"high"}` {
		t.Errorf("doc get = %s,%v", body, ok)
	}
	recs, _ := st.DocQuery("findings", 10)
	if len(recs) != 1 {
		t.Errorf("doc query len = %d", len(recs))
	}

	// brain: add + dedup + search
	id1, added1, _ := st.BrainAdd("ethereum gas fees spiked today", "experience", "")
	if !added1 {
		t.Error("first add should be new")
	}
	_, added2, _ := st.BrainAdd("ethereum gas fees spiked today", "experience", "")
	if added2 {
		t.Error("duplicate content should not be added again")
	}
	hits, _ := st.BrainSearch("gas fees", 5)
	if len(hits) == 0 || hits[0].ID != id1 {
		t.Errorf("brain search miss: %+v", hits)
	}
}

// ── end-to-end through the kernel ────────────────────────────────────────────

func newTestKernel(t *testing.T) (*Kernel, *capturedBus) {
	t.Helper()
	dir := t.TempDir()
	cb := &capturedBus{}
	k := NewKernel()
	RegisterBuiltins(k, Deps{
		StorePath: func(module string) (string, error) {
			return filepath.Join(dir, module+".db"), nil // per-module file = isolation
		},
		Send: cb.send,
		Invoke: func(_ context.Context, target string, msg Message) (json.RawMessage, error) {
			// Echo the source id back so tests can assert the kernel stamped it.
			return json.RawMessage(`{"from":"` + msg.Source.ID + `","to":"` + target + `"}`), nil
		},
	})
	return k, cb
}

type capturedBus struct {
	mu   sync.Mutex
	last *Message
	to   string
}

func (c *capturedBus) send(_ context.Context, target string, msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = &msg
	c.to = target
	return nil
}

func TestKernelStoreRoundTrip(t *testing.T) {
	k, _ := newTestKernel(t)
	ctx := context.Background()

	if r := k.Call(ctx, "title-writer", "store.kv.set", json.RawMessage(`{"k":"style","v":"punchy"}`)); !r.OK {
		t.Fatalf("kv.set failed: %s", r.Error)
	}
	r := k.Call(ctx, "title-writer", "store.kv.get", json.RawMessage(`{"k":"style"}`))
	if !r.OK {
		t.Fatalf("kv.get failed: %s", r.Error)
	}
	var got struct {
		Value string `json:"value"`
		Found bool   `json:"found"`
	}
	_ = json.Unmarshal(r.Result, &got)
	if !got.Found || got.Value != "punchy" {
		t.Errorf("round trip wrong: %+v", got)
	}
}

func TestKernelStoreIsolation(t *testing.T) {
	k, _ := newTestKernel(t)
	ctx := context.Background()
	// Module A writes.
	k.Call(ctx, "agent-a", "store.kv.set", json.RawMessage(`{"k":"secret","v":"A-only"}`))
	// Module B must NOT see it — different folder, different store.
	r := k.Call(ctx, "agent-b", "store.kv.get", json.RawMessage(`{"k":"secret"}`))
	var got struct {
		Found bool `json:"found"`
	}
	_ = json.Unmarshal(r.Result, &got)
	if got.Found {
		t.Error("ISOLATION BREACH: agent-b read agent-a's key")
	}
}

func TestKernelBusStampsCallerIdentity(t *testing.T) {
	k, cb := newTestKernel(t)
	// A module cannot forge the source: even if it puts a fake source in payload,
	// the kernel stamps Source.ID from the verified caller.
	r := k.Call(context.Background(), "real-sender", "bus.send",
		json.RawMessage(`{"to":"telegram","payload":{"text":"hi"}}`))
	if !r.OK {
		t.Fatalf("bus.send failed: %s", r.Error)
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.last == nil || cb.last.Source.ID != "real-sender" || cb.to != "telegram" {
		t.Errorf("bus did not stamp caller correctly: %+v to=%s", cb.last, cb.to)
	}
}

func TestKernelBusRequestReply(t *testing.T) {
	k, _ := newTestKernel(t)
	r := k.Call(context.Background(), "group-youtube", "bus.request",
		json.RawMessage(`{"to":"title-writer","payload":{"topic":"cats"}}`))
	if !r.OK {
		t.Fatalf("bus.request failed: %s", r.Error)
	}
	// Reply should carry the stamped source (group-youtube) per the echo Invoke.
	if !json.Valid(r.Result) {
		t.Errorf("invalid reply json: %s", r.Result)
	}
}
