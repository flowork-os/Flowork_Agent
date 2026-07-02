// detect_filter_ext.go — SIBLING ext CLITool filter (⚠️ FROZEN 2026-07-02 seizin
// owner — behavior stabil dikunci). 📄 Dok: lock/connect-prune.md
// Rapihin daftar CLI Tools (owner: "CLI untested ga bisa dites, sisain yg proven").
// Sembunyiin tool yg BENER-BENER ga ada (not-installed & ga ke-config); simpen yg
// installed/connected/ke-config. Switch env FLOWORK_CLITOOL_PRUNE=0 → tampilin semua
// (ubah behavior TANPA buka gembok). Colok filter lain: bikin sibling _ext.go baru.
package clitools

import "os"

func init() {
	DetectFilter = func(in []Status) []Status {
		if v := os.Getenv("FLOWORK_CLITOOL_PRUNE"); v == "0" || v == "false" {
			return in
		}
		out := make([]Status, 0, len(in))
		for _, s := range in {
			// Tampilin kalau ADA jejak: binari keinstall, file setting ada, atau
			// udah ke-route Flowork. Pure not-found (untested) → sembunyiin.
			if s.Installed || s.SettingsExists || s.HasFlowRouter || s.TokenSet {
				out = append(out, s)
			}
		}
		return out
	}
}
