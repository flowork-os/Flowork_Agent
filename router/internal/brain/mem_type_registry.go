// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package brain

import "strings"

type MemType string

const (
	MemTypeUser      MemType = "user"
	MemTypeFeedback  MemType = "feedback"
	MemTypeProject   MemType = "project"
	MemTypeReference MemType = "reference"

	MemTypeExperience MemType = "experience"
	MemTypeEureka     MemType = "eureka"

	MemTypeFact     MemType = "fact"
	MemTypeAntibody MemType = "antibody"

	MemTypeDoctrine MemType = "doctrine"
	MemTypeSkill    MemType = "skill"

	MemTypeRecoveryInstinct    MemType = "recovery_instinct"
	MemTypeCollectiveKnowledge MemType = "collective_knowledge"
)

var AllMemTypes = []MemType{
	MemTypeUser, MemTypeFeedback, MemTypeProject, MemTypeReference,
	MemTypeExperience, MemTypeEureka,
	MemTypeFact, MemTypeAntibody,
	MemTypeDoctrine, MemTypeSkill,
	MemTypeRecoveryInstinct, MemTypeCollectiveKnowledge,
}

var validSet map[MemType]struct{}

func init() {
	validSet = make(map[MemType]struct{}, len(AllMemTypes))
	for _, mt := range AllMemTypes {
		validSet[mt] = struct{}{}
	}
}

func IsValid(s string) bool {
	_, ok := validSet[MemType(s)]
	return ok
}

func Validate(s string) (MemType, bool) {
	mt := MemType(s)
	_, ok := validSet[mt]
	if !ok {
		return "", false
	}
	return mt, true
}

var legacyMap = map[string]MemType{

	"knowledge":        MemTypeReference,
	"drawer":           MemTypeProject,
	"public_knowledge": MemTypeReference,

	"compounding": MemTypeProject,

	"ref":          MemTypeReference,
	"refs":         MemTypeReference,
	"usr":          MemTypeUser,
	"exp":          MemTypeExperience,
	"fb":           MemTypeFeedback,
	"proj":         MemTypeProject,
	"anti":         MemTypeAntibody,
	"doc":          MemTypeDoctrine,
	"constitution": MemTypeDoctrine,
}

func MapLegacy(s string) MemType {

	if mt, ok := Validate(s); ok {
		return mt
	}

	lower := strings.ToLower(strings.TrimSpace(s))
	if mt, ok := legacyMap[lower]; ok {
		return mt
	}

	return MemTypeProject
}

func Promotable(mt MemType) bool {
	switch mt {
	case MemTypeExperience, MemTypeEureka, MemTypeFact:
		return true
	default:
		return false
	}
}

func FreshIndexable(mt MemType) bool {
	switch mt {
	case MemTypeRecoveryInstinct, MemTypeCollectiveKnowledge:
		return true
	default:
		return false
	}
}

func Sacred(mt MemType) bool {
	switch mt {
	case MemTypeUser, MemTypeDoctrine, MemTypeAntibody:
		return true
	default:
		return false
	}
}

func GUIOptions() []MemType {
	return []MemType{
		MemTypeProject,
		MemTypeReference,
		MemTypeFeedback,
		MemTypeUser,
		MemTypeDoctrine,
		MemTypeExperience,
		MemTypeFact,
		MemTypeAntibody,
		MemTypeSkill,
		MemTypeEureka,
		MemTypeRecoveryInstinct,
		MemTypeCollectiveKnowledge,
	}
}

func (mt MemType) String() string {
	return string(mt)
}
