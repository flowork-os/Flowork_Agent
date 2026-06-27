// taskflow_ext.go — TITIK REGISTRASI (NON-FROZEN, BISA DIHAPUS) buat strategi eksekusi crew.
//
// ⚖️ ATURAN ABADI (owner Mr.Dev): file yg udah di-FREEZE TIDAK BOLEH dibuka lagi buat nambah filtur.
// Mode eksekusi crew baru (parallel/debate/vote/dll) DAFTAR via RegisterExecStrategy — taskflow.go
// (frozen) cuma manggil extRunCategory() sekali. MEKANISME-nya beku di taskflow_seam.go; di SINI
// (atau file sibling baru) tinggal init(){ RegisterExecStrategy(...) }. Hapus file ini → ga ada
// strategi tambahan → default frozen sequential (RunCategoryTask) yg KEBUKTI. Inti ga patah.
//
// Contoh nambah strategi (bikin file sibling baru atau init di sini):
//   func init() { RegisterExecStrategy(myParallelStrategy) }
//
// 📖 WAJIB BACA: FLowork_os/lock/group.md sebelum ngutak-atik.
package taskflow
