// === LOCKED FILE (soft) === Status: STABLE (owner-approved 2026-06-15, R4). E2E verified:
// router brain enrich inject skill lewat registry provider (log: "enriched … 2 skills").
// LOCKED≠FREEZE. Extend = RegisterSkillProvider, jangan edit core.
//
// skill_provider.go — R4 EXTENSION POINT: sumber skill yang PLUG-ABLE.
// Owner-approved 2026-06-15 (refactor konsolidasi R4). Dulu: allSkills() hardcode
// embedded + dynamic-dir. Sekarang: REGISTRY provider — nambah sumber skill (remote,
// DB, app-bundle, …) = RegisterSkillProvider, allSkills() merge semua. Builtin (embedded
// library + runtime-authored on-disk) di-register di init(). Behavior identik pra-R4.
package brain

// SkillProvider nyuplai SkillDoc dari SATU sumber (embedded, on-disk, remote, …).
// Dipanggil tiap allSkills() (jadi provider on-disk/remote selalu fresh per-call).
type SkillProvider func() []SkillDoc

// skillProviders = rantai sumber skill. Urutan = presedensi: provider lebih AWAL menang
// saat nama bentrok. Diisi init() (builtin) + RegisterSkillProvider (eksternal/runtime).
var skillProviders []SkillProvider

// RegisterSkillProvider nambah sumber skill ke akhir rantai. Titik-extend resmi R4:
// nambah sumber TANPA edit allSkills/core (sejalan prinsip anti-edit-core).
func RegisterSkillProvider(p SkillProvider) {
	if p != nil {
		skillProviders = append(skillProviders, p)
	}
}

func init() {
	// Builtin: embedded library DULU (menang saat bentrok), lalu runtime-authored on-disk.
	RegisterSkillProvider(Skills)
	RegisterSkillProvider(loadDynamicSkills)
}
