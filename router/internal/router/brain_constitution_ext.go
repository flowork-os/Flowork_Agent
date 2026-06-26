// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package router

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

var constitutionFilterHook func(context.Context, []brain.ConstitutionEntry) []brain.ConstitutionEntry

func init() { constitutionFilterHook = externalConstitutionFilter }

var internalDoctrines = map[string]bool{
	"AOLA-001": true,
	"AOLA-002": true,
	"AOLA-006": true,
	"AOLA-007": true,
	"AOLA-008": true,
	"AOLA-012": true,
}

var aolaIDRe = regexp.MustCompile(`AOLA-\d{3}`)

func externalConstitutionFilter(ctx context.Context, rules []brain.ConstitutionEntry) []brain.ConstitutionEntry {
	if AgentIDFromContext(ctx) != "" || !brainExternalScopeEnabled() {
		return rules
	}
	out := make([]brain.ConstitutionEntry, 0, len(rules))
	dropped := 0
	for _, r := range rules {
		id := aolaIDRe.FindString(r.Section)
		if id == "" {
			id = aolaIDRe.FindString(r.Content)
		}
		if id != "" && internalDoctrines[strings.ToUpper(id)] {
			dropped++
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return rules
	}
	return out
}
