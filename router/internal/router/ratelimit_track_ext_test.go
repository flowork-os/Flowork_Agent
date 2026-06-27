package router

import (
	"net/http"
	"testing"
)

func TestRateLimitTrack_ParseHeaders(t *testing.T) {
	rlState = RateLimitState{} // reset state share
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-5h-utilization", "0.87")
	h.Set("anthropic-ratelimit-unified-5h-reset", "1782560000")
	h.Set("anthropic-ratelimit-unified-5h-surpassed-threshold", "false")
	h.Set("anthropic-ratelimit-unified-7d-utilization", "42") // format 0..100 → 0.42
	h.Set("anthropic-ratelimit-unified-fallback-percentage", "12.5")
	trackAnthropicRateLimit(h)

	st := RateLimitSnapshot()
	if !st.Seen {
		t.Fatal("expected Seen=true")
	}
	if st.Util5h != 0.87 {
		t.Errorf("util_5h = %v, want 0.87", st.Util5h)
	}
	if st.Util7d != 0.42 {
		t.Errorf("util_7d = %v, want 0.42 (normalisasi 42→0.42)", st.Util7d)
	}
	if st.Surpassed5h {
		t.Error("surpassed_5h should be false")
	}
	if st.FallbackPct != 12.5 {
		t.Errorf("fallback_pct = %v, want 12.5", st.FallbackPct)
	}
}

func TestRateLimitTrack_NoHeaderIgnored(t *testing.T) {
	rlState = RateLimitState{}
	trackAnthropicRateLimit(http.Header{}) // ga ada header kuota
	if RateLimitSnapshot().Seen {
		t.Fatal("provider non-langganan (no header) ga boleh nge-set Seen")
	}
}

func TestSubscriptionNearLimit(t *testing.T) {
	rlState = RateLimitState{}
	if SubscriptionNearLimit() {
		t.Fatal("belum ada data → near-limit harus false")
	}
	rlState = RateLimitState{Seen: true, Util5h: 0.5}
	if SubscriptionNearLimit() {
		t.Fatal("0.5 < 0.95 → false")
	}
	rlState = RateLimitState{Seen: true, Util5h: 0.97}
	if !SubscriptionNearLimit() {
		t.Fatal("0.97 >= 0.95 → true")
	}
	rlState = RateLimitState{Seen: true, Util5h: 0.1, Surpassed5h: true}
	if !SubscriptionNearLimit() {
		t.Fatal("surpassed=true → true (apapun util)")
	}
}
