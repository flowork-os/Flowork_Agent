// toolsidecar_ext.go — CABANG/SWITCH (NON-frozen) buat KEBIJAKAN toolsidecar.go yg FROZEN.
//
// ⭐ JALAN PINTAS: mau ubah kebijakan keamanan tool buatan-agent (denylist import) atau nambah
// kebijakan baru TANPA buka `toolsidecar.go` (frozen brain-core)? EDIT DI SINI. File ini sengaja
// dibiarin non-frozen — itu yg bikin engine sidecar tetap "abadi" tapi kebijakannya bisa berevolusi.
//
// Kenapa import-policy yang dicabangin: ini bagian yg PALING mungkin berubah seiring waktu —
//   - mau izinin import yg dulu ditolak (mis. `math/rand`)? buang dari daftar.
//   - mau tutup vektor eskalasi baru? tambah ke daftar.
//   - nanti ada sandbox-OS (seccomp) → denylist ini bisa dilonggarin (lihat lock/tools.md §guardrail).
//
// Arsitektur + alasan lengkap: lock/tools.md. Owner: Mr.Dev · github.com/flowork-os/Flowork-OS
package toolsidecar

// dangerImports — denylist vektor eskalasi native yg DITOLAK pas `tool_create` (Fase 1, sebelum
// sandbox-OS ada). Heuristik substring, BUKAN sandbox beneran; lapis aman utama = scope PRIVAT +
// Dewan-review sebelum shared (lock/tools.md §guardrail). `CreateTool` di toolsidecar.go baca var ini.
//
// SWITCH: tambah/buang entri di sini buat ngatur kebijakan — JANGAN sentuh toolsidecar.go.
var dangerImports = []string{"os/exec", "syscall", "unsafe", "plugin", "net/http/cgi", "runtime/debug"}
