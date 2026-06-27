// groupsapi_ext.go — TITIK REGISTRASI (NON-FROZEN, BISA DIHAPUS) buat hook sync group.
//
// ⚖️ ATURAN ABADI (owner Mr.Dev): file frozen TIDAK BOLEH dibuka buat nambah filtur. Target sync
// BARU (menu Discord/WhatsApp, registry eksternal, metrik) daftar via RegisterGroupSyncHook — file
// frozen cuma MANGGIL. MEKANISME + switch-reader beku di groupsapi_seam.go; di SINI (atau sibling
// baru) tinggal init(){ RegisterGroupSyncHook(...) }. Hapus file ini → ga ada hook tambahan → sync
// jalan apa adanya (inti ga patah, delete-test §6.4 lulus).
//
// Contoh nambah hook (bikin file sibling baru atau init di sini):
//   func init() { RegisterGroupSyncHook(myDiscordSyncHook) }
//
// 📖 WAJIB BACA: FLowork_os/lock/group.md sebelum ngutak-atik soal group.
package groupsapi
