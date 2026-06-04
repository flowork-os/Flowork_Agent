package taskflow

import (
	"context"
	"strings"
	"testing"
)

func TestParseRetask(t *testing.T) {
	cases := []struct {
		in       string
		wantOK   bool
		wantRole string
	}{
		{"RETASK Analis Fundamental: cari BBCA bukan BBNI", true, "Analis Fundamental"},
		{"**RETASK Fundamental: data salah ticker**", true, "Fundamental"},
		{"> RETASK Keuangan: minta angka ROE BBCA", true, "Keuangan"},
		{"KEPUTUSAN: BUY\nALASAN: solid", false, ""},
		{"retask tanpa colon jadi ga valid", false, ""},
	}
	for _, c := range cases {
		role, instr, ok := parseRetask(c.in)
		if ok != c.wantOK {
			t.Fatalf("parseRetask(%q) ok=%v want %v", c.in, ok, c.wantOK)
		}
		if ok && role != c.wantRole {
			t.Fatalf("parseRetask(%q) role=%q want %q", c.in, role, c.wantRole)
		}
		if ok && instr == "" {
			t.Fatalf("parseRetask(%q) instruksi kosong", c.in)
		}
	}
}

func TestFindCrewByRole(t *testing.T) {
	crew := []CrewMember{
		{AgentID: "saham-fundamental", RoleLabel: "Analis Fundamental"},
		{AgentID: "saham-teknikal", RoleLabel: "Analis Teknikal"},
	}
	if m := findCrewByRole(crew, "Analis Fundamental"); m == nil || m.AgentID != "saham-fundamental" {
		t.Fatal("exact match gagal")
	}
	if m := findCrewByRole(crew, "fundamental"); m == nil || m.AgentID != "saham-fundamental" {
		t.Fatal("contains match gagal")
	}
	if m := findCrewByRole(crew, "Tidak Ada"); m != nil {
		t.Fatal("role ga ada harusnya nil (stop retask)")
	}
}

// stub Invoker: simulasi synth nyuruh RETASK → worker dikoreksi → synth kasih vonis.
type stubInvoker struct {
	synthCalls int
	calls      []string
}

func (s *stubInvoker) InvokeAgentMessage(ctx context.Context, agentID, text, caller string) (string, error) {
	s.calls = append(s.calls, agentID)
	switch agentID {
	case "synth":
		s.synthCalls++
		if s.synthCalls == 1 {
			return "RETASK Fundamental: cari data BBCA (Bank Central Asia), BUKAN BBNI", nil
		}
		return "KEPUTUSAN: BUY\nALASAN: data BBCA solid", nil
	case "fundamental":
		if strings.Contains(text, "KOREKSI WAJIB") {
			return "Analisa BBCA (Bank Central Asia) — fundamental bagus", nil // sudah benar
		}
		return "Analisa BBNI — (salah ticker)", nil // awal: salah
	default:
		return "output " + agentID, nil
	}
}

func TestRunCategoryTask_SelfHealRetask(t *testing.T) {
	host := &stubInvoker{}
	cat := Category{
		ID: "saham", Name: "Saham", Synthesizer: "synth",
		Crew: []CrewMember{
			{AgentID: "fundamental", RoleLabel: "Fundamental"},
			{AgentID: "teknikal", RoleLabel: "Teknikal"},
		},
	}
	res := RunCategoryTask(context.Background(), host, t.TempDir(), cat, "BBCA", "99", nil)

	if strings.Contains(res.Recommendation, "RETASK") {
		t.Fatalf("output final masih RETASK — loop self-heal ga jalan: %q", res.Recommendation)
	}
	if !strings.Contains(res.Recommendation, "BUY") {
		t.Fatalf("output final harusnya keputusan (post-retask), dapet: %q", res.Recommendation)
	}
	if host.synthCalls != 2 {
		t.Fatalf("synth harus 2x (awal + ulang), dapet %d", host.synthCalls)
	}
	fc := 0
	for _, a := range host.calls {
		if a == "fundamental" {
			fc++
		}
	}
	if fc != 2 {
		t.Fatalf("fundamental harus 2x (awal + TUGAS ULANG), dapet %d", fc)
	}
}
