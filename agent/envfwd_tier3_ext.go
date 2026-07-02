// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// 📄 Dok: FLowork_os/lock/prompt-diet.md (§F-E breakpoint ke-4)
//
// envfwd_tier3_ext.go — SIBLING ext (deletable, non-frozen): forward switch
// FLOWORK_TIER3_TAIL ke guest WASM (dibaca mr-flow main.go tier3ToTail()).
// Nempel di papan RegisterEnvForward (envfwd_seam.go, frozen — Pola A).
// Hapus file ini → switch ga ke-forward → guest pakai default (ON).
package main

func init() {
	RegisterEnvForward(func(string) []string {
		return []string{
			"FLOWORK_TIER3_TAIL", // mr-flow: Tier-3 volatile ke ekor messages (cache bp-4)
		}
	})
}
