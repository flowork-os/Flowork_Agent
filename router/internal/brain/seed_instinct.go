// seed_instinct.go — INSTINCT + PERSONA SEED (NON-frozen sibling seed_doctrine.go). Bawa SOUL
// basic ke tiap install biar user juga dapet (owner 2026-06-25: "flowork gw gratisin, persona+
// cara-pikir Aola di-lock as-is; versi corporate white-label dijual"). Versi FREE = brand Aola.
//
// Isi: 282 insting (room instinct_*: universal/coding/security/crypto/tool + kesadaran) + persona
// default (identitas Mr.Flow + pencipta Aola = BRANDING, sengaja). Embedded di binary → ship ke
// OS/USB/Android tanpa file eksternal. Idempotent: no-op begitu brain udah punya insting/persona.
//
// ⚠️ PRIVASI (owner-defined): nama PENCIPTA (Aola) = branding, BOLEH publik. Yang HARAM = secret
// (password/key → never di brain) + nama PIHAK-KETIGA (anak/teman). Room `knowledge` (history Aola
// sama teman) di-BERSIHIN dari publik — NOT di-seed. instinct_seed.json hanya insting generic +
// persona (di-scan: 0 secret-value, 0 nama-pihak-ketiga). Konstitusi 12 AOLA = via seed_doctrine.

package brain

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed instinct_seed.json
var instinctSeedJSON []byte

type instinctSeedEntry struct {
	Content string `json:"content"`
	Room    string `json:"room"`
	Wing    string `json:"wing"`
}

type personaSeedEntry struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type instinctSeedFile struct {
	Instincts []instinctSeedEntry `json:"instincts"`
	Personas  []personaSeedEntry  `json:"personas"`
}

// SeedInstincts — isi brain fresh dgn insting basic + persona default embedded. Balik (added, error)
// dimana added = jumlah insting. Idempotent: insting di-seed cuma kalau brain belum punya insting;
// persona di-seed cuma kalau prompt_templates kosong (jangan clobber yg owner-edited).
func SeedInstincts() (int, error) {
	if _, err := EnsureSchema(); err != nil {
		return 0, fmt.Errorf("seed instinct: schema: %w", err)
	}
	db, err := OpenRW()
	if err != nil {
		return 0, fmt.Errorf("seed instinct: open: %w", err)
	}
	var seed instinctSeedFile
	if err := json.Unmarshal(instinctSeedJSON, &seed); err != nil {
		return 0, fmt.Errorf("seed instinct: parse embed: %w", err)
	}
	ctx := context.Background()

	// 1) insting — seed kalau belum ada insting hidup
	var ni int
	_ = db.QueryRow(`SELECT COUNT(*) FROM drawers WHERE room LIKE 'instinct\_%' ESCAPE '\' AND deleted_at IS NULL`).Scan(&ni)
	added := 0
	if ni == 0 {
		for _, it := range seed.Instincts {
			if !strings.HasPrefix(it.Room, "instinct_") {
				continue // guard privasi/konvensi: cuma room instinct_*
			}
			wing := it.Wing
			if wing == "" {
				wing = "doctrine"
			}
			if _, _, err := AddDrawer(ctx, it.Content, wing, it.Room, "project"); err == nil {
				added++
			}
		}
	}

	// 2) persona — seed kalau prompt_templates masih kosong (fresh install)
	var np int
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates`).Scan(&np)
	if np == 0 {
		for _, p := range seed.Personas {
			if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Content) == "" {
				continue
			}
			_ = AddPersona(ctx, p.Name, p.Content, "flowork-seed")
		}
	}

	return added, nil
}
