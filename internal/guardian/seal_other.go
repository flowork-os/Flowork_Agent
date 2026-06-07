//go:build !linux && !darwin && !windows

package guardian

import "errors"

// osSealer (OS lain: *BSD/dll) — noop. Guardian tetap jalan dalam detection-only (FASE 1):
// hash-compare nangkep tamper, cuma ga mencegah tulis. Tambah dukungan = tambah seal_<os>.go.
func osSealer() Sealer { return noopSealer{} }

type noopSealer struct{}

func (noopSealer) Name() string                    { return "noop (OS belum didukung)" }
func (noopSealer) Seal(string) error               { return errors.New("seal: OS ini belum didukung (detection-only)") }
func (noopSealer) Unseal(string) error             { return nil }
func (noopSealer) IsSealed(string) (bool, error)   { return false, nil }
