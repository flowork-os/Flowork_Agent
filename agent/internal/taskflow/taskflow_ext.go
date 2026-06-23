// taskflow_ext.go — CABANG (extension point) NON-FROZEN buat taskflow CORE yang
// FROZEN (taskflow.go, taskflow_retask.go).
//
// ⚖️ ATURAN ABADI (owner Mr.Dev, 2026-06-23): file yang udah di-FREEZE TIDAK BOLEH
// dibuka lagi buat nambah filtur. Mode eksekusi crew baru (parallel/debate/vote/dll)
// DAFTAR di sini lewat RegisterExecStrategy — taskflow.go (frozen) cuma manggil
// extRunCategory() sekali, ga pernah dirombak lagi. Default frozen = sequential
// fan-out → synth (RunCategoryTask), yang udah KEBUKTI (gate saham).
//
// 📖 WAJIB BACA: /home/mrflow/Documents/FLowork_os/lock/group.md sebelum ngutak-atik.
package taskflow

import "context"

// ExecStrategy — strategi eksekusi crew ALTERNATIF buat 1 Category. Default frozen =
// sequential (RunCategoryTask). Mode masa depan (parallel/debate/vote) daftar strategi
// di sini: kalau dia KLAIM kategorinya (balik non-nil) → engine pake itu; balik nil =
// "bukan punya gue, lanjut ke default sequential".
type ExecStrategy func(ctx context.Context, host Invoker, sharedDir string, cat Category, input, runID string, rec Recorder) *Result

var execStrategies []ExecStrategy

// RegisterExecStrategy — daftarin strategi (panggil dari init() file NON-frozen).
// Strategi pertama yang balik non-nil menang; urutan = urutan daftar.
func RegisterExecStrategy(fn ExecStrategy) {
	if fn != nil {
		execStrategies = append(execStrategies, fn)
	}
}

// extRunCategory — kasih kesempatan strategi terdaftar ambil-alih 1 run. Balik nil →
// default frozen (sequential) yang nanganin. Dipanggil di awal RunCategoryTask.
// Strategi yang panic ga boleh ngerusak run (recover → lanjut default).
func extRunCategory(ctx context.Context, host Invoker, sharedDir string, cat Category, input, runID string, rec Recorder) (out *Result) {
	for _, fn := range execStrategies {
		func() {
			defer func() { _ = recover() }()
			if r := fn(ctx, host, sharedDir, cat, input, runID, rec); r != nil {
				out = r
			}
		}()
		if out != nil {
			return out
		}
	}
	return nil
}
