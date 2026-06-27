package main

import "testing"

func TestNerveProposal_Channels(t *testing.T) {
	cases := []struct {
		kind        string
		wantChannel string
		wantAllowed bool
	}{
		{"add-skill", "data", true},
		{"add-agent", "modul", true},
		{"add-app", "modul", true},
		{"fix", "core-edit", false},      // edit kode inti → TOLAK
		{"refactor", "core-edit", false}, // edit kode inti → TOLAK
		{"doc", "core-edit", false},
		{"test", "core-edit", false},
		{"hapus-semua", "unknown", false}, // ngaco → TOLAK
	}
	for _, c := range cases {
		v := NerveProposalVet(c.kind, "NEW:whatever.go")
		if v.Channel != c.wantChannel || v.Allowed != c.wantAllowed {
			t.Errorf("kind=%q → channel=%q allowed=%v; mau channel=%q allowed=%v (reason=%q)",
				c.kind, v.Channel, v.Allowed, c.wantChannel, c.wantAllowed, v.Reason)
		}
	}
}

func TestNerveProposal_SwitchMustExist(t *testing.T) {
	// pencet saklar yg TERDAFTAR → boleh.
	if v := NerveProposalVet("set-switch", "switch:FLOWORK_EDITION"); !v.Allowed {
		t.Fatalf("saklar terdaftar harus boleh, dapat %+v", v)
	}
	// pencet saklar NGARANG → tolak (anti-halu).
	if v := NerveProposalVet("set-switch", "switch:FLOWORK_NGACO_XYZ"); v.Allowed {
		t.Fatalf("saklar ga terdaftar harus DITOLAK, dapat %+v", v)
	}
}
