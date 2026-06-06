package scanner

import (
	"fmt"
	"testing"
)

// Guard wiring: pipa utuh → 0 finding; pola dicabut → CRITICAL; file ilang → CRITICAL.
func TestCheckInvariants_IntactNoFinding(t *testing.T) {
	invs := []wiringInvariant{
		{relPath: "a.go", mustHave: []string{"maybeInjectAntibodies", "rankAntibodies"}, reason: "x"},
	}
	read := func(rel string) (string, error) {
		return "func maybeInjectAntibodies(){}\nfunc rankAntibodies(){}", nil
	}
	if got := checkInvariants(invs, read); len(got) != 0 {
		t.Fatalf("pipa utuh harusnya 0 finding, dapet %d: %+v", len(got), got)
	}
}

func TestCheckInvariants_PatternRemovedIsCritical(t *testing.T) {
	invs := []wiringInvariant{
		{relPath: "dispatcher.go", mustHave: []string{"maybeInjectAntibodies"}, reason: "hook antibody"},
	}
	// Simulasi AI nyabut hook.
	read := func(rel string) (string, error) { return "func dispatch(){ /* hook dicabut */ }", nil }
	got := checkInvariants(invs, read)
	if len(got) != 1 {
		t.Fatalf("pola dicabut harusnya 1 CRITICAL, dapet %d", len(got))
	}
	if got[0].Severity != SevCritical {
		t.Fatalf("harus CRITICAL, dapet %v", got[0].Severity)
	}
}

func TestCheckInvariants_MissingFileIsCritical(t *testing.T) {
	invs := []wiringInvariant{{relPath: "gone.go", mustHave: []string{"x"}, reason: "r"}}
	read := func(rel string) (string, error) { return "", fmt.Errorf("no such file") }
	got := checkInvariants(invs, read)
	if len(got) != 1 || got[0].Severity != SevCritical {
		t.Fatalf("file ilang harusnya 1 CRITICAL, dapet %+v", got)
	}
}

// Registry asli: tiap entri minimal punya path + 1 pola + alasan (anti entri kosong).
func TestWiringInvariants_RegistryWellFormed(t *testing.T) {
	if len(wiringInvariants) == 0 {
		t.Fatal("registry kosong — minimal jaga pipa antibody + deterministic route")
	}
	for i, inv := range wiringInvariants {
		if inv.relPath == "" || len(inv.mustHave) == 0 || inv.reason == "" {
			t.Fatalf("invariant #%d malformed: %+v", i, inv)
		}
	}
}
