// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package safego

import (
	"log"
	"runtime/debug"
)

func Go(fn func()) {
	GoLabel("", fn)
}

func GoLabel(label string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if label == "" {
					log.Printf("safego: recovered panic: %v\n%s", r, debug.Stack())
					return
				}
				log.Printf("safego[%s]: recovered panic: %v\n%s", label, r, debug.Stack())
			}
		}()
		fn()
	}()
}
