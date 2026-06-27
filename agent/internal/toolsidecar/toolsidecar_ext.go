// toolsidecar_ext.go — TITIK OVERRIDE (NON-frozen, BISA DIHAPUS) buat kebijakan toolsidecar.go (frozen).
//
// ⭐ JALAN PINTAS: mau ubah kebijakan keamanan tool buatan-agent (denylist import) TANPA buka
// `toolsidecar.go` (frozen brain-core)? OVERRIDE DI SINI lewat init() — JANGAN edit toolsidecar_seam.go.
// File ini sengaja non-frozen biar kebijakan bisa berevolusi. Hapus file ini → balik ke DEFAULT BEKU
// (toolsidecar_seam.go) yg aman — inti ga patah (delete-test §6.4 lulus).
//
//   - mau izinin import yg dulu ditolak (mis. `math/rand`)? override: buang dari daftar.
//   - mau tutup vektor eskalasi baru? override: tambah ke daftar.
//   - nanti ada sandbox-OS (seccomp) → denylist ini bisa dilonggarin (lihat lock/tools.md §guardrail).
//
// Contoh override (uncomment + sesuaikan):
//   func init() { dangerImports = []string{"os/exec", "syscall", "unsafe", "plugin"} }
//
// Default aman ada di toolsidecar_seam.go (BEKU). Arsitektur: lock/tools.md. Owner: Mr.Dev.
package toolsidecar
