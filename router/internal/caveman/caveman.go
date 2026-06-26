// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package caveman

import "strings"

type Level string

const (
	LevelOff   Level = ""
	LevelLite  Level = "lite"
	LevelFull  Level = "full"
	LevelUltra Level = "ultra"
)

const sharedBoundaries = "Code blocks, file paths, commands, errors, URLs: keep exact. " +
	"Security warnings, irreversible action confirmations, multi-step ordered sequences: write normal. " +
	"Resume terse style after."

var prompts = map[Level]string{
	LevelLite: strings.Join([]string{
		"Respond tersely. Keep grammar and full sentences but drop filler, hedging and pleasantries (just/really/basically/sure/of course/I'd be happy to).",
		"Pattern: state the thing, the action, the reason. Then next step.",
		sharedBoundaries,
		"Active every response until user asks for normal mode.",
	}, " "),

	LevelFull: strings.Join([]string{
		"Respond like terse caveman. All technical substance stay exact, only fluff die.",
		"Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries, hedging. Fragments OK. Short synonyms (big not extensive, fix not implement a solution for).",
		"Pattern: [thing] [action] [reason]. [next step].",
		sharedBoundaries,
		"Active every response until user asks for normal mode.",
	}, " "),

	LevelUltra: strings.Join([]string{
		"Respond ultra-terse. Maximum compression. Telegraphic.",
		"Abbreviate (DB/auth/config/req/res/fn/impl), strip conjunctions, use arrows for causality (X → Y). One word when one word enough.",
		"Pattern: [thing] → [result]. [fix].",
		sharedBoundaries,
		"Active every response until user asks for normal mode.",
	}, " "),
}

func Prompt(l Level) string {
	return prompts[l]
}

func Normalize(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "lite":
		return LevelLite
	case "full":
		return LevelFull
	case "ultra":
		return LevelUltra
	default:
		return LevelOff
	}
}

func InjectIntoSystem(existing, prompt string) string {
	if prompt == "" {
		return existing
	}
	if existing == "" {
		return prompt
	}
	return existing + "\n\n" + prompt
}
