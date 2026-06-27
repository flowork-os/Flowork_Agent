// 📄 Dok: FLowork_os/lock/group.md
//
// taskflow_seam.go — MEKANISME PAPAN BEKU (POLA-A) buat taskflow.go (frozen). Dispatcher strategi
// eksekusi crew. Pisahan dari taskflow_ext.go (non-frozen, titik REGISTRASI): mekanisme + default
// KOSONG aman ADA DI SINI biar inti beku self-sufficient (delete-test §6.4: hapus file registrasi →
// execStrategies kosong → default sequential, build OK).
//
// Strategi BARU: JANGAN edit file ini. Daftar via init(){ RegisterExecStrategy(...) } di file SIBLING
// BARU (mis. taskflow_parallel.go). Default frozen = sequential (RunCategoryTask) — KEBUKTI.
package taskflow

import "context"

// ExecStrategy — strategi eksekusi crew ALTERNATIF buat 1 Category. Default frozen =
// sequential (RunCategoryTask). Strategi yg KLAIM kategorinya (balik non-nil) → engine pake itu;
// balik nil = "bukan punya gue, lanjut ke default sequential".
type ExecStrategy func(ctx context.Context, host Invoker, sharedDir string, cat Category, input, runID string, rec Recorder) *Result

var execStrategies []ExecStrategy

// RegisterExecStrategy — daftarin strategi (panggil dari init() file SIBLING BARU, non-frozen).
// Strategi pertama yg balik non-nil menang; urutan = urutan daftar.
func RegisterExecStrategy(fn ExecStrategy) {
	if fn != nil {
		execStrategies = append(execStrategies, fn)
	}
}

// extRunCategory — kasih kesempatan strategi terdaftar ambil-alih 1 run. Balik nil →
// default frozen (sequential) yg nanganin. Dipanggil di awal RunCategoryTask.
// Strategi yg panic ga boleh ngerusak run (recover → lanjut default).
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
