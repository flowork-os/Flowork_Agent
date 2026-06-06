package agentmgr

import "testing"

func TestAgentTier(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"mr-flow", "primary"},
		{"MR-FLOW", "primary"},   // case-insensitive
		{" mr-flow ", "primary"}, // trimmed
		{"crypto-fundamental", "extension"},
		{"music-judul", "extension"},
		{"zodiak-peramal", "extension"},
		{"saham-teknikal", "extension"},
		{"", "extension"}, // unknown → extension (default aman: folder brain)
	}
	for _, c := range cases {
		if got := AgentTier(c.id); got != c.want {
			t.Errorf("AgentTier(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestIsPrimaryAgent(t *testing.T) {
	if !IsPrimaryAgent("mr-flow") {
		t.Error("mr-flow harus primary")
	}
	if IsPrimaryAgent("crypto-fundamental") {
		t.Error("crypto-fundamental harus BUKAN primary (extension)")
	}
}

func TestIsPrimaryOnlyTool(t *testing.T) {
	if !IsPrimaryOnlyTool("brain_search_shared") {
		t.Error("brain_search_shared harus primary-only (korpus 5jt)")
	}
	// brain lokal = SEMUA tier (folder sendiri) — JANGAN ke-gate.
	if IsPrimaryOnlyTool("brain_search") {
		t.Error("brain_search (lokal) JANGAN primary-only — semua agent punya brain folder sendiri")
	}
	if IsPrimaryOnlyTool("brain_add") {
		t.Error("brain_add (lokal) JANGAN primary-only")
	}
	if IsPrimaryOnlyTool("file_read") {
		t.Error("file_read tool umum, bukan primary-only")
	}
}

// TestTierGateDecision — simulasi keputusan gate persis di ToolRunHandler:
// refuse kalau primary-only tool dipanggil agent non-primary.
func TestTierGateDecision(t *testing.T) {
	refused := func(toolName, agentID string) bool {
		return IsPrimaryOnlyTool(toolName) && !IsPrimaryAgent(agentID)
	}
	// extension panggil 5jt → DITOLAK.
	if !refused("brain_search_shared", "crypto-fundamental") {
		t.Error("extension panggil brain_search_shared HARUS ditolak")
	}
	// primary panggil 5jt → LOLOS.
	if refused("brain_search_shared", "mr-flow") {
		t.Error("primary (mr-flow) panggil brain_search_shared JANGAN ditolak")
	}
	// extension panggil brain lokal → LOLOS (folder sendiri, ga di-gate).
	if refused("brain_search", "crypto-fundamental") {
		t.Error("extension panggil brain_search (lokal) JANGAN ditolak — brain folder sendiri")
	}
	// extension panggil tool umum → LOLOS.
	if refused("file_read", "crypto-fundamental") {
		t.Error("extension panggil file_read JANGAN ditolak")
	}
}
