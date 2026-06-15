// selfevolve_apply_test.go — R7 fase-2b: bukti deterministik GATE behavior-apply (manual path).
// Batas keamanan inti: mode=off ATAU model lemah/lokal HARUS ngeblok apply. Manual path
// (owner klik) ga nyentuh DB karma (cuma saklar + model) → unit-test bersih tanpa network/DB.

package agentmgr

import "testing"

func TestEvolveBehaviorApplyAllowed_Manual(t *testing.T) {
	mk := func(mode string, strong bool) EvolveGateDeps {
		return EvolveGateDeps{
			KVGet: func(k string) (string, error) {
				if k == "evolve_mode" {
					return mode, nil
				}
				return "", nil
			},
			ModelStrong: func() (bool, string) { return strong, "test" },
		}
	}
	cases := []struct {
		name   string
		mode   string
		strong bool
		want   bool
	}{
		{"off blocks", "off", true, false},
		{"empty defaults off blocks", "", true, false},
		{"garbage mode treated as off", "banana", true, false},
		{"stage+strong allows", "stage", true, true},
		{"stage+weak/local blocks", "stage", false, false},
		{"auto+strong allows manual", "auto", true, true},
		{"auto+weak blocks", "auto", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := EvolveBehaviorApplyAllowed(mk(c.mode, c.strong), false)
			if got != c.want {
				t.Fatalf("mode=%q strong=%v: got allowed=%v (%q), want %v", c.mode, c.strong, got, why, c.want)
			}
		})
	}
}

// core-apply gate (🔴): edisi WAJIB dev; public selalu block. Manual path ga nyentuh DB karma.
func TestEvolveCoreApplyAllowed_Manual(t *testing.T) {
	mk := func(edition, mode string, strong bool) EvolveGateDeps {
		return EvolveGateDeps{
			KVGet:       func(k string) (string, error) { return mode, nil },
			ModelStrong: func() (bool, string) { return strong, "test" },
			Edition:     func() string { return edition },
		}
	}
	cases := []struct {
		name    string
		edition string
		mode    string
		strong  bool
		want    bool
	}{
		{"public always blocks (even armed+strong)", "public", "auto", true, false},
		{"public stage blocks", "public", "stage", true, false},
		{"dev off blocks", "dev", "off", true, false},
		{"dev stage weak blocks", "dev", "stage", false, false},
		{"dev stage strong allows", "dev", "stage", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := EvolveCoreApplyAllowed(mk(c.edition, c.mode, c.strong), false)
			if got != c.want {
				t.Fatalf("edition=%s mode=%s strong=%v: got %v (%q) want %v", c.edition, c.mode, c.strong, got, why, c.want)
			}
		})
	}
}

// nil ModelStrong (dep belum lengkap) tetap aman: jangan fail-OPEN. mode off → block.
func TestEvolveBehaviorApplyAllowed_NilModelStrong(t *testing.T) {
	dep := EvolveGateDeps{
		KVGet: func(k string) (string, error) { return "stage", nil },
		// ModelStrong nil → guard anti-lokal ga jalan, tapi mode stage → manual allowed.
	}
	if ok, _ := EvolveBehaviorApplyAllowed(dep, false); !ok {
		t.Fatal("stage + nil ModelStrong manual: expected allowed (no model guard wired)")
	}
	depOff := EvolveGateDeps{KVGet: func(k string) (string, error) { return "off", nil }}
	if ok, why := EvolveBehaviorApplyAllowed(depOff, false); ok {
		t.Fatalf("off + nil ModelStrong: expected BLOCKED, got allowed (%q)", why)
	}
}
