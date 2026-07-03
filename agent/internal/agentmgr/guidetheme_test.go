package agentmgr

import "testing"

// TestEvolveGuideThemeCollapse — FIX theme-collapse: varian morfologis "reflection" (reflection/
// refleksi/reflective + prefix beda) harus kolaps ke tema stem yang SAMA biar guideThemeCap nangkep,
// bukan dianggap tema beda (dulu ambil kata-pertama → lolos → 9/12 usulan tema sama).
func TestEvolveGuideThemeCollapse(t *testing.T) {
	// reflection variants → stem 'refle' (kata terpanjang = reflection/reflective).
	refl := []string{
		"NEW:reflection-mistake-triage",
		"NEW:scheduled-reflection",
		"NEW:warfare-reflection-guardrail",
		"NEW:skill_reflective_autonomy",
		"NEW:contekan-reflection-gate",
	}
	for _, c := range refl {
		if th := evolveGuideTheme(c, "add-skill"); th != "refle" {
			t.Errorf("%s → %q, mau 'refle' (kolaps reflection)", c, th)
		}
	}
	// tema BEDA harus tetep beda (jangan over-merge sampe matiin diversitas).
	if a, b := evolveGuideTheme("NEW:web-anti-block", ""), evolveGuideTheme("NEW:shell-guard", ""); a == b {
		t.Errorf("tema beda ke-merge: web-anti-block=%q shell-guard=%q", a, b)
	}
	// path core → basename stem, bukan kosong.
	if th := evolveGuideTheme("internal/tools/builtins/shell_guard.go", "refactor"); th == "" {
		t.Errorf("path core → tema kosong")
	}
}
