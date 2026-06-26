// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

type SkillProvider func() []SkillDoc

var skillProviders []SkillProvider

func RegisterSkillProvider(p SkillProvider) {
	if p != nil {
		skillProviders = append(skillProviders, p)
	}
}

func init() {

	RegisterSkillProvider(Skills)
	RegisterSkillProvider(loadDynamicSkills)
}
