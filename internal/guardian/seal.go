// seal.go — ROADMAP Guardian FASE 2 (L3): OS-Immutability Adapter.
//
// Sealer = primitive tunggal "bikin file tak-tertulis (immutable) di tingkat OS". Implementasi
// 1 file per-OS (build tag) — filosofi pasukan semut, plug-and-play: tambah OS = tambah 1 file,
// nol perubahan inti. Yang disegel = binary + manifest freeze + vault (BUKAN source kernel —
// itu cukup dideteksi via hash di FASE 1; nge-seal .go source malah ngancurin build).
//
// Butuh privilege (root/admin) — Seal gagal kalau ga ada → guardian DEGRADE ke detection-only
// (FASE 1) dgn warning, BUKAN crash. Jadi `sudo flowork --arm` = full immutable; tanpa root =
// deteksi doang.
package guardian

import (
	"os"
	"path/filepath"
)

// Sealer — adapter immutability OS. IsSealed dipakai status/GUI; Seal/Unseal saat arm/disarm.
type Sealer interface {
	Seal(path string) error
	Unseal(path string) error
	IsSealed(path string) (bool, error)
	Name() string
}

// activeSealer — di-set per-OS via osSealer() (seal_<os>.go). Bisa di-override test.
var activeSealer = osSealer()

// DefaultSealer — adapter immutability untuk OS ini.
func DefaultSealer() Sealer { return activeSealer }

// setSealerForTest — ganti sealer (unit test, hindari chattr asli).
func setSealerForTest(s Sealer) func() {
	prev := activeSealer
	activeSealer = s
	return func() { activeSealer = prev }
}

// immutableTargets — file yang dijadikan immutable saat arm: binary (otak — kernel ada DI sini)
// + manifest freeze (biar ga bisa ditukar). Vault disegel TERPISAH (terakhir saat arm, pertama
// saat disarm). Source .go kernel TIDAK disegel (cukup deteksi hash) — biar build dev ga rusak.
func immutableTargets() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		if r, e := filepath.EvalSymlinks(exe); e == nil {
			exe = r
		}
		out = append(out, exe)
	}
	if _, err := os.Stat("KERNEL_FREEZE.md"); err == nil {
		if abs, e := filepath.Abs("KERNEL_FREEZE.md"); e == nil {
			out = append(out, abs)
		}
	}
	return out
}
