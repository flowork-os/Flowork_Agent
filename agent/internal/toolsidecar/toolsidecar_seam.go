// 📄 Dok: FLowork_os/lock/tools.md
//
// toolsidecar_seam.go — DEFAULT BEKU (papan POLA-B) buat kebijakan toolsidecar.go (frozen).
// Pisahan dari toolsidecar_ext.go (non-frozen, titik OVERRIDE): default aman ADA DI SINI biar
// inti beku self-sufficient (delete-test §6.4: hapus _ext → default seam ini tetep nahan, build OK).
//
// Customisasi denylist: JANGAN edit file ini — override di toolsidecar_ext.go (non-frozen) via
// init(){ dangerImports = ... }. Hapus _ext → balik ke default aman di bawah.
package toolsidecar

// dangerImports — denylist vektor eskalasi native yg DITOLAK pas `tool_create` (Fase 1, sebelum
// sandbox-OS ada). Heuristik substring, BUKAN sandbox beneran; lapis aman utama = scope PRIVAT +
// Dewan-review sebelum shared (lock/tools.md §guardrail). `CreateTool` di toolsidecar.go baca var ini.
// DEFAULT BEKU — override (tambah/buang) lewat toolsidecar_ext.go.
var dangerImports = []string{"os/exec", "syscall", "unsafe", "plugin", "net/http/cgi", "runtime/debug"}
