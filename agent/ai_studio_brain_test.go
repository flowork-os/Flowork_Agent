package main

import (
	"encoding/json"
	"testing"
)

// TestExtractDesignJSON — D31 reason-first: parser WAJIB ambil objek JSON SPEC dari reply
// agent yang boleh ngandung reasoning + code-fence, TANPA ke-racun '{' di teks reasoning.
func TestExtractDesignJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // hasil yang diharapkan (di-normalize via unmarshal->marshal)
		ok   bool   // true = harus valid JSON object
	}{
		{
			name: "plain json (model lemah langsung emit)",
			in:   `{"group_id":"peramal","x":1}`,
			want: `{"group_id":"peramal","x":1}`,
			ok:   true,
		},
		{
			name: "fenced json block",
			in:   "Ini desainnya:\n```json\n{\"group_id\":\"kuliner\"}\n```\n",
			want: `{"group_id":"kuliner"}`,
			ok:   true,
		},
		{
			name: "reason-first lalu fenced (Opus mikir dulu)",
			in:   "Owner mau tim peramal. Aku pikir butuh 2 spesialis {primbon, fengshui} + 1 lead.\nFinal:\n```json\n{\"group_id\":\"peramal\",\"n\":2}\n```",
			want: `{"group_id":"peramal","n":2}`,
			ok:   true,
		},
		{
			name: "reason dgn brace lalu json TANPA fence (balanced-last)",
			in:   "Pertimbangan: struktur {seperti ini} kurang pas.\nHasil akhir:\n{\"group_id\":\"final\",\"ok\":true}",
			want: `{"group_id":"final","ok":true}`,
			ok:   true,
		},
		{
			name: "nested object diambil utuh",
			in:   "```json\n{\"lead\":{\"name\":\"Utama\"},\"group_id\":\"x\"}\n```",
			want: `{"lead":{"name":"Utama"},"group_id":"x"}`,
			ok:   true,
		},
		{
			name: "no json sama sekali",
			in:   "maaf gw ga ngerti",
			ok:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractDesignJSON(c.in)
			var m map[string]any
			err := json.Unmarshal([]byte(got), &m)
			if c.ok {
				if err != nil {
					t.Fatalf("expected valid JSON, got parse error: %v (extracted=%q)", err, got)
				}
				// bandingin via normalisasi: unmarshal want juga
				var w map[string]any
				if e := json.Unmarshal([]byte(c.want), &w); e != nil {
					t.Fatalf("bad want literal: %v", e)
				}
				gb, _ := json.Marshal(m)
				wb, _ := json.Marshal(w)
				if string(gb) != string(wb) {
					t.Fatalf("mismatch:\n got=%s\nwant=%s", gb, wb)
				}
			} else {
				if err == nil {
					t.Fatalf("expected NON-json (fallback), but parsed ok: %q", got)
				}
			}
		})
	}
}
