package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestIsCtxExceeded_RealError — pakai error PERSIS dari screenshot owner (2026-06-21): router 502
// membungkus upstream 500 "Context size has been exceeded". Bukti deteksi recovery bakal nyala.
func TestIsCtxExceeded_RealError(t *testing.T) {
	real := fmt.Errorf(`router status 502: {"error":{"message":"all providers failed; last error: upstream 500: {\"error\":{\"code\":500,\"message\":\"Context size has been exceeded.\",\"type\":\"server_error\"}}","type":"router_error"}}`)
	if !isCtxExceeded(real) {
		t.Fatal("isCtxExceeded HARUS true buat error context-exceeded dari router")
	}
	if isCtxExceeded(fmt.Errorf("router status 429: rate limit exceeded")) {
		t.Fatal("isCtxExceeded HARUS false buat rate-limit (bukan context)")
	}
	if isCtxExceeded(nil) {
		t.Fatal("nil HARUS false")
	}
}

// TestCompactMessages_DropMiddle — context kegedean krn turn TENGAH (tool-dump raksasa): compact
// buang tengah, simpan system[0] + task terakhir, hasil <= budget.
func TestCompactMessages_DropMiddle(t *testing.T) {
	big := strings.Repeat("x", 400000) // ~100k token tool-dump di tengah
	msgs := []map[string]any{
		{"role": "system", "content": "you are studio"},
		{"role": "user", "content": "turn lama 1"},
		{"role": "assistant", "content": big},
		{"role": "user", "content": "task terakhir penting"},
	}
	budget := 7000
	before := estTokens(msgs)
	out, changed := compactMessages(msgs, budget)
	after := estTokens(out)
	if !changed {
		t.Fatal("HARUS changed (context kegedean)")
	}
	if after > budget {
		t.Fatalf("compact GAGAL muat budget: after=%d budget=%d", after, budget)
	}
	if out[0]["content"] != "you are studio" {
		t.Fatal("system (msg[0]) HARUS dipertahankan")
	}
	if last, _ := out[len(out)-1]["content"].(string); !strings.Contains(last, "task terakhir") {
		t.Fatalf("user terakhir (task inti) HARUS dipertahankan, dapet: %q", last)
	}
	t.Logf("drop-middle: %d tok -> %d tok (budget %d), msgs %d -> %d", before, after, budget, len(msgs), len(out))
}

// TestCompactMessages_TruncateHugeLast — kasus EVOLUSI: pesan terakhir (kode) sendiri kegedean →
// di-truncate + ditandai (model lihat marker, bukan korupsi diam-diam), hasil <= budget.
func TestCompactMessages_TruncateHugeLast(t *testing.T) {
	huge := strings.Repeat("CODE", 100000) // ~100k token di pesan terakhir
	msgs := []map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": huge},
	}
	budget := 7000
	out, changed := compactMessages(msgs, budget)
	if !changed {
		t.Fatal("HARUS changed")
	}
	if after := estTokens(out); after > budget {
		t.Fatalf("HARUS muat budget, dapet %d > %d", after, budget)
	}
	if s, _ := out[1]["content"].(string); !strings.Contains(s, "dipotong demi context budget") {
		t.Fatal("pesan terakhir raksasa HARUS di-truncate + ditandai")
	}
}

// TestCtxBudget — model cloud context besar, model lokal konservatif.
func TestCtxBudget(t *testing.T) {
	if ctxBudgetTokens("claude-opus-4-8") < 100000 {
		t.Fatal("opus harus budget besar (>=100k)")
	}
	if ctxBudgetTokens("flowork-brain") > 20000 {
		t.Fatal("model lokal harus konservatif")
	}
}
